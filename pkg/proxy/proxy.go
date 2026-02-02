// Package proxy provides the gRPC proxy server that intercepts Temporal traffic.
package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/temporal-profiling/temporal-profiler/pkg/buffer"
	"github.com/temporal-profiling/temporal-profiler/pkg/config"
)

// Server is the gRPC proxy server.
type Server struct {
	config       config.ProxyConfig
	grpcServer   *grpc.Server
	upstreamConn *grpc.ClientConn
	interceptor  *ProfilingInterceptor
	logger       *zap.Logger

	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new proxy server.
func NewServer(cfg config.ProxyConfig, buf buffer.Buffer, logger *zap.Logger) (*Server, error) {
	s := &Server{
		config:      cfg,
		interceptor: NewProfilingInterceptor(buf, logger),
		logger:      logger,
	}

	return s, nil
}

// Start starts the proxy server.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// Connect to upstream Temporal server
	upstreamOpts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(100*1024*1024), // 100MB
			grpc.MaxCallSendMsgSize(100*1024*1024), // 100MB
		),
	}

	if s.config.TLS.Enabled {
		tlsConfig, err := s.loadUpstreamTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to load upstream TLS config: %w", err)
		}
		upstreamOpts = append(upstreamOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		upstreamOpts = append(upstreamOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	var err error
	s.upstreamConn, err = grpc.NewClient(s.config.UpstreamAddr, upstreamOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to upstream: %w", err)
	}

	// Create gRPC server with interceptors
	serverOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(100 * 1024 * 1024), // 100MB
		grpc.MaxSendMsgSize(100 * 1024 * 1024), // 100MB
		grpc.ChainUnaryInterceptor(s.interceptor.UnaryInterceptor(), s.unaryProxy()),
		grpc.ChainStreamInterceptor(s.interceptor.StreamInterceptor(), s.streamProxy()),
		grpc.UnknownServiceHandler(s.unknownServiceHandler()),
	}

	if s.config.TLS.Enabled {
		tlsConfig, err := s.loadServerTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to load server TLS config: %w", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	s.grpcServer = grpc.NewServer(serverOpts...)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s.grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start listening
	s.listener, err = net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}

	s.logger.Info("proxy server starting",
		zap.String("listen", s.config.ListenAddr),
		zap.String("upstream", s.config.UpstreamAddr),
	)

	// Start serving in a goroutine
	go func() {
		if err := s.grpcServer.Serve(s.listener); err != nil {
			s.logger.Error("grpc server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully stops the proxy server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false

	// Graceful shutdown with timeout
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("proxy server stopped gracefully")
	case <-ctx.Done():
		s.grpcServer.Stop()
		s.logger.Warn("proxy server force stopped")
	}

	if s.upstreamConn != nil {
		s.upstreamConn.Close()
	}

	return nil
}

// unaryProxy returns an interceptor that forwards unary calls to upstream.
func (s *Server) unaryProxy() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// This interceptor is chained after profiling, so just forward
		return handler(ctx, req)
	}
}

// streamProxy returns an interceptor that forwards stream calls to upstream.
func (s *Server) streamProxy() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// This interceptor is chained after profiling, so just forward
		return handler(srv, ss)
	}
}

// unknownServiceHandler handles all unknown services by proxying to upstream.
func (s *Server) unknownServiceHandler() grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		method, ok := grpc.MethodFromServerStream(stream)
		if !ok {
			return fmt.Errorf("failed to get method from stream")
		}

		// Create context with timeout
		ctx, cancel := context.WithTimeout(stream.Context(), 5*time.Minute)
		defer cancel()

		// Create client stream to upstream
		clientStream, err := s.upstreamConn.NewStream(ctx, &grpc.StreamDesc{
			StreamName:    method,
			ServerStreams: true,
			ClientStreams: true,
		}, method)
		if err != nil {
			return fmt.Errorf("failed to create upstream stream: %w", err)
		}

		// Bidirectional proxy
		errChan := make(chan error, 2)

		// Forward client -> upstream
		go func() {
			for {
				msg := new([]byte)
				if err := stream.RecvMsg(msg); err != nil {
					clientStream.CloseSend()
					errChan <- err
					return
				}
				if err := clientStream.SendMsg(msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		// Forward upstream -> client
		go func() {
			for {
				msg := new([]byte)
				if err := clientStream.RecvMsg(msg); err != nil {
					errChan <- err
					return
				}
				if err := stream.SendMsg(msg); err != nil {
					errChan <- err
					return
				}
			}
		}()

		// Wait for either direction to complete
		err = <-errChan
		if err.Error() == "EOF" {
			return nil
		}
		return err
	}
}

// loadUpstreamTLSConfig loads TLS configuration for connecting to upstream.
func (s *Server) loadUpstreamTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if s.config.TLS.CAFile != "" {
		caCert, err := os.ReadFile(s.config.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if s.config.TLS.CertFile != "" && s.config.TLS.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLS.CertFile, s.config.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// loadServerTLSConfig loads TLS configuration for the proxy server.
func (s *Server) loadServerTLSConfig() (*tls.Config, error) {
	if s.config.TLS.CertFile == "" || s.config.TLS.KeyFile == "" {
		return nil, fmt.Errorf("TLS cert and key files required for server TLS")
	}

	cert, err := tls.LoadX509KeyPair(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if s.config.TLS.CAFile != "" {
		caCert, err := os.ReadFile(s.config.TLS.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsConfig, nil
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.config.ListenAddr
}
