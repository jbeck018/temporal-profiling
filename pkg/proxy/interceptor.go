package proxy

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/temporal-profiling/temporal-profiler/pkg/buffer"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
)

// ProfilingInterceptor intercepts gRPC calls to extract profiling data.
type ProfilingInterceptor struct {
	buffer     buffer.Buffer
	classifier *profiler.Classifier
	logger     *zap.Logger
}

// NewProfilingInterceptor creates a new profiling interceptor.
func NewProfilingInterceptor(buf buffer.Buffer, logger *zap.Logger) *ProfilingInterceptor {
	return &ProfilingInterceptor{
		buffer:     buf,
		classifier: profiler.NewClassifier(),
		logger:     logger,
	}
}

// UnaryInterceptor returns a gRPC unary server interceptor for profiling.
func (p *ProfilingInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Check if we should profile this method
		if !p.classifier.ShouldProfile(info.FullMethod) {
			return handler(ctx, req)
		}

		// Start timing
		start := time.Now()

		// Extract request metadata (non-blocking)
		reqMeta := ExtractMetadata(ctx, req, info.FullMethod)

		// Forward to upstream
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Extract response metadata
		var respMeta *ResponseMetadata
		if resp != nil {
			respMeta = ExtractResponseMetadata(resp, info.FullMethod)
		}

		// Create profile event
		event := p.createEvent(info.FullMethod, reqMeta, respMeta, start, duration, err)

		// Non-blocking push to buffer
		if !p.buffer.Push(event) {
			// Event dropped (buffer full) - this is acceptable for zero overhead
			p.logger.Debug("event dropped", zap.String("method", info.FullMethod))
		}

		return resp, err
	}
}

// StreamInterceptor returns a gRPC stream server interceptor for profiling.
func (p *ProfilingInterceptor) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Check if we should profile this method
		if !p.classifier.ShouldProfile(info.FullMethod) {
			return handler(srv, ss)
		}

		// Start timing
		start := time.Now()

		// Create wrapped stream to intercept messages
		wrapped := &profilingServerStream{
			ServerStream: ss,
			interceptor:  p,
			method:       info.FullMethod,
			start:        start,
		}

		// Forward to upstream
		err := handler(srv, wrapped)

		// Calculate duration
		duration := time.Since(start)

		// Create profile event for the overall stream
		event := &profiler.ProfileEvent{
			Timestamp:     start,
			Duration:      duration,
			EventType:     p.classifier.Classify(info.FullMethod),
			OperationType: info.FullMethod,
			Status:        p.deriveStatus(err),
		}

		if err != nil {
			event.ErrorMessage = err.Error()
			event.ErrorType = status.Code(err).String()
		}

		// Non-blocking push
		p.buffer.Push(event)

		return err
	}
}

// createEvent creates a ProfileEvent from intercepted call data.
func (p *ProfilingInterceptor) createEvent(
	method string,
	reqMeta *RequestMetadata,
	respMeta *ResponseMetadata,
	start time.Time,
	duration time.Duration,
	err error,
) *profiler.ProfileEvent {
	event := &profiler.ProfileEvent{
		Timestamp:     start,
		Duration:      duration,
		EventType:     p.classifier.Classify(method),
		OperationType: method,
		Status:        p.deriveStatus(err),
	}

	// Populate from request metadata
	if reqMeta != nil {
		event.WorkflowID = reqMeta.WorkflowID
		event.RunID = reqMeta.RunID
		event.WorkflowType = reqMeta.WorkflowType
		event.ActivityID = reqMeta.ActivityID
		event.ActivityType = reqMeta.ActivityType
		event.TaskQueue = reqMeta.TaskQueue
		event.Namespace = reqMeta.Namespace
		event.Attempt = reqMeta.Attempt
	}

	// Enrich from response metadata
	if respMeta != nil {
		if event.RunID == "" {
			event.RunID = respMeta.RunID
		}
		if event.WorkflowID == "" {
			event.WorkflowID = respMeta.WorkflowID
		}
		if event.WorkflowType == "" {
			event.WorkflowType = respMeta.WorkflowType
		}
		if event.ActivityType == "" {
			event.ActivityType = respMeta.ActivityType
		}
		if event.ActivityID == "" {
			event.ActivityID = respMeta.ActivityID
		}
		if event.Attempt == 0 {
			event.Attempt = respMeta.Attempt
		}
	}

	// Add error information
	if err != nil {
		event.ErrorMessage = err.Error()
		event.ErrorType = status.Code(err).String()
	}

	return event
}

// deriveStatus converts a gRPC error to a profiler Status.
func (p *ProfilingInterceptor) deriveStatus(err error) profiler.Status {
	if err == nil {
		return profiler.StatusOK
	}

	code := status.Code(err)
	switch code {
	case codes.OK:
		return profiler.StatusOK
	case codes.Canceled:
		return profiler.StatusCanceled
	case codes.DeadlineExceeded:
		return profiler.StatusTimeout
	default:
		return profiler.StatusError
	}
}

// profilingServerStream wraps a grpc.ServerStream to intercept streaming messages.
type profilingServerStream struct {
	grpc.ServerStream
	interceptor *ProfilingInterceptor
	method      string
	start       time.Time
	msgCount    int
}

// RecvMsg intercepts incoming stream messages.
func (s *profilingServerStream) RecvMsg(m interface{}) error {
	err := s.ServerStream.RecvMsg(m)
	s.msgCount++
	return err
}

// SendMsg intercepts outgoing stream messages.
func (s *profilingServerStream) SendMsg(m interface{}) error {
	return s.ServerStream.SendMsg(m)
}
