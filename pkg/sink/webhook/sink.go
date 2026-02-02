// Package webhook provides a generic webhook sink for sending events to HTTP endpoints.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
)

// WebhookPayload represents the payload sent to webhooks.
type WebhookPayload struct {
	Timestamp time.Time                `json:"timestamp"`
	Events    []*profiler.ProfileEvent `json:"events"`
	Count     int                      `json:"count"`
}

// Sink sends profiling events to a webhook endpoint.
type Sink struct {
	*sink.BaseSink

	config     *config.WebhookSinkConfig
	logger     *zap.Logger
	httpClient *http.Client

	// Batching
	batch []*profiler.ProfileEvent
	mu    sync.Mutex
}

// NewSink creates a new webhook sink.
func NewSink(cfg *config.WebhookSinkConfig, logger *zap.Logger) *Sink {
	return &Sink{
		BaseSink: sink.NewBaseSink("webhook"),
		config:   cfg,
		logger:   logger,
		batch:    make([]*profiler.ProfileEvent, 0, cfg.BatchSize),
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Init initializes the webhook sink.
func (s *Sink) Init(ctx context.Context) error {
	s.logger.Info("webhook sink initialized",
		zap.String("url", s.config.URL),
		zap.String("method", s.config.Method),
		zap.Int("batch_size", s.config.BatchSize),
	)
	return nil
}

// Write writes events to the webhook.
func (s *Sink) Write(ctx context.Context, events []*profiler.ProfileEvent) error {
	s.mu.Lock()
	s.batch = append(s.batch, events...)

	// Send if batch is full
	if len(s.batch) >= s.config.BatchSize {
		batch := s.batch
		s.batch = make([]*profiler.ProfileEvent, 0, s.config.BatchSize)
		s.mu.Unlock()
		return s.send(ctx, batch)
	}

	s.mu.Unlock()
	return nil
}

// send sends a batch of events to the webhook.
func (s *Sink) send(ctx context.Context, events []*profiler.ProfileEvent) error {
	if len(events) == 0 {
		return nil
	}

	payload := WebhookPayload{
		Timestamp: time.Now(),
		Events:    events,
		Count:     len(events),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, s.config.Method, s.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.SetHealth(sink.HealthDegraded)
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.SetHealth(sink.HealthDegraded)
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	s.SetHealth(sink.HealthOK)
	s.logger.Debug("sent webhook batch", zap.Int("count", len(events)))

	return nil
}

// Flush flushes any pending events.
func (s *Sink) Flush(ctx context.Context) error {
	s.mu.Lock()
	batch := s.batch
	s.batch = make([]*profiler.ProfileEvent, 0, s.config.BatchSize)
	s.mu.Unlock()

	if len(batch) > 0 {
		return s.send(ctx, batch)
	}
	return nil
}

// Close closes the sink.
func (s *Sink) Close(ctx context.Context) error {
	return s.Flush(ctx)
}
