// Package admin provides the admin HTTP server for health checks and metrics.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
)

// Server is the admin HTTP server.
type Server struct {
	config     config.AdminConfig
	logger     *zap.Logger
	httpServer *http.Server
	pipeline   PipelineStats

	mu      sync.RWMutex
	ready   bool
	started time.Time
}

// PipelineStats provides access to pipeline statistics.
type PipelineStats interface {
	Stats() sink.Stats
}

// NewServer creates a new admin server.
func NewServer(cfg config.AdminConfig, logger *zap.Logger) *Server {
	return &Server{
		config:  cfg,
		logger:  logger,
		started: time.Now(),
	}
}

// SetPipeline sets the pipeline for statistics.
func (s *Server) SetPipeline(p PipelineStats) {
	s.pipeline = p
}

// SetReady marks the server as ready.
func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = ready
}

// Start starts the admin server.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc(s.config.Endpoints.Health, s.handleHealth)

	// Ready endpoint
	mux.HandleFunc(s.config.Endpoints.Ready, s.handleReady)

	// Metrics endpoint (Prometheus format)
	mux.Handle(s.config.Endpoints.Metrics, promhttp.Handler())

	// Stats endpoint (JSON)
	mux.HandleFunc("/stats", s.handleStats)

	s.httpServer = &http.Server{
		Addr:    s.config.Addr,
		Handler: mux,
	}

	go func() {
		s.logger.Info("admin server starting", zap.String("addr", s.config.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("admin server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the admin server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleHealth handles health check requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Uptime:    time.Since(s.started).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleReady handles readiness check requests.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not_ready"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// handleStats handles statistics requests.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var stats StatsResponse

	if s.pipeline != nil {
		pipelineStats := s.pipeline.Stats()
		stats = StatsResponse{
			EventsProcessed: pipelineStats.EventsProcessed,
			BatchesSent:     pipelineStats.BatchesSent,
			Errors:          pipelineStats.Errors,
			BufferSize:      pipelineStats.BufferStats.Size,
			BufferCapacity:  pipelineStats.BufferStats.Capacity,
			BufferDropped:   pipelineStats.BufferStats.Dropped,
			SinkHealth:      make(map[string]string),
		}

		for name, health := range pipelineStats.SinkHealth {
			stats.SinkHealth[name] = health.String()
		}
	}

	stats.Uptime = time.Since(s.started).String()
	stats.Timestamp = time.Now().UTC()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HealthResponse is the response for health checks.
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Uptime    string    `json:"uptime"`
}

// StatsResponse is the response for statistics.
type StatsResponse struct {
	EventsProcessed int64             `json:"events_processed"`
	BatchesSent     int64             `json:"batches_sent"`
	Errors          int64             `json:"errors"`
	BufferSize      int64             `json:"buffer_size"`
	BufferCapacity  int64             `json:"buffer_capacity"`
	BufferDropped   int64             `json:"buffer_dropped"`
	SinkHealth      map[string]string `json:"sink_health"`
	Uptime          string            `json:"uptime"`
	Timestamp       time.Time         `json:"timestamp"`
}

// Metrics registers Prometheus metrics.
type Metrics struct {
	EventsProcessed prometheus.Counter
	EventsDropped   prometheus.Counter
	BatchesSent     prometheus.Counter
	Errors          prometheus.Counter
	BufferSize      prometheus.Gauge
	ProxyLatency    prometheus.Histogram
}

// NewMetrics creates and registers Prometheus metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		EventsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "temporal_profiler",
			Name:      "events_processed_total",
			Help:      "Total number of events processed",
		}),
		EventsDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "temporal_profiler",
			Name:      "events_dropped_total",
			Help:      "Total number of events dropped due to buffer overflow",
		}),
		BatchesSent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "temporal_profiler",
			Name:      "batches_sent_total",
			Help:      "Total number of batches sent to sinks",
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "temporal_profiler",
			Name:      "errors_total",
			Help:      "Total number of errors",
		}),
		BufferSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "temporal_profiler",
			Name:      "buffer_size",
			Help:      "Current number of events in buffer",
		}),
		ProxyLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "temporal_profiler",
			Name:      "proxy_latency_seconds",
			Help:      "Latency of proxy operations",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
		}),
	}

	prometheus.MustRegister(
		m.EventsProcessed,
		m.EventsDropped,
		m.BatchesSent,
		m.Errors,
		m.BufferSize,
		m.ProxyLatency,
	)

	return m
}

// Addr returns the admin server address.
func (s *Server) Addr() string {
	return fmt.Sprintf("http://%s", s.config.Addr)
}
