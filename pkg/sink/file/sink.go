// Package file provides a file sink for writing profiling events to disk.
package file

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
)

// Sink writes profiling events to a file in JSON Lines format.
type Sink struct {
	*sink.BaseSink

	config  *config.FileSinkConfig
	logger  *zap.Logger
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
	mu      sync.Mutex
}

// NewSink creates a new file sink.
func NewSink(cfg *config.FileSinkConfig, logger *zap.Logger) *Sink {
	return &Sink{
		BaseSink: sink.NewBaseSink("file"),
		config:   cfg,
		logger:   logger,
	}
}

// Init initializes the file sink.
func (s *Sink) Init(ctx context.Context) error {
	// Ensure directory exists
	dir := filepath.Dir(s.config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Open file for appending
	file, err := os.OpenFile(s.config.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", s.config.Path, err)
	}

	s.file = file
	s.writer = bufio.NewWriter(file)
	s.encoder = json.NewEncoder(s.writer)

	s.logger.Info("file sink initialized", zap.String("path", s.config.Path))

	return nil
}

// Write writes events to the file.
func (s *Sink) Write(ctx context.Context, events []*profiler.ProfileEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range events {
		if err := s.encoder.Encode(event); err != nil {
			s.SetHealth(sink.HealthDegraded)
			return fmt.Errorf("failed to encode event: %w", err)
		}
	}

	return nil
}

// Flush flushes the buffer to disk.
func (s *Sink) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush writer: %w", err)
		}
	}

	if s.file != nil {
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("failed to sync file: %w", err)
		}
	}

	return nil
}

// Close closes the file sink.
func (s *Sink) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		s.writer.Flush()
	}

	if s.file != nil {
		if err := s.file.Close(); err != nil {
			return fmt.Errorf("failed to close file: %w", err)
		}
	}

	s.logger.Info("file sink closed")

	return nil
}
