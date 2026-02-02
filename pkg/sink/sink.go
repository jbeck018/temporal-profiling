// Package sink provides output sink interfaces and implementations for profiling events.
package sink

import (
	"context"

	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
)

// HealthStatus represents the health state of a sink.
type HealthStatus int

const (
	HealthOK HealthStatus = iota
	HealthDegraded
	HealthUnhealthy
)

// String returns the string representation of HealthStatus.
func (h HealthStatus) String() string {
	switch h {
	case HealthOK:
		return "ok"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Sink is the interface that all output destinations must implement.
type Sink interface {
	// Name returns the unique identifier for this sink.
	Name() string

	// Init initializes the sink with its configuration.
	Init(ctx context.Context) error

	// Write writes a batch of events to the sink.
	Write(ctx context.Context, events []*profiler.ProfileEvent) error

	// Flush ensures all pending data is written.
	Flush(ctx context.Context) error

	// Close gracefully shuts down the sink.
	Close(ctx context.Context) error

	// Health returns the current health status of the sink.
	Health() HealthStatus
}

// Processor is the interface for event processors that transform events
// before they are written to sinks.
type Processor interface {
	// Name returns the processor name.
	Name() string

	// Process transforms an event. Returns nil to filter out the event.
	Process(event *profiler.ProfileEvent) (*profiler.ProfileEvent, error)
}

// FilterFunc is a function that determines if an event should be processed.
type FilterFunc func(event *profiler.ProfileEvent) bool

// FilterProcessor filters events based on a filter function.
type FilterProcessor struct {
	name   string
	filter FilterFunc
}

// NewFilterProcessor creates a new filter processor.
func NewFilterProcessor(name string, filter FilterFunc) *FilterProcessor {
	return &FilterProcessor{name: name, filter: filter}
}

// Name returns the processor name.
func (f *FilterProcessor) Name() string {
	return f.name
}

// Process filters events.
func (f *FilterProcessor) Process(event *profiler.ProfileEvent) (*profiler.ProfileEvent, error) {
	if f.filter(event) {
		return event, nil
	}
	return nil, nil // Filter out
}

// TransformFunc is a function that transforms an event.
type TransformFunc func(event *profiler.ProfileEvent) *profiler.ProfileEvent

// TransformProcessor transforms events using a transform function.
type TransformProcessor struct {
	name      string
	transform TransformFunc
}

// NewTransformProcessor creates a new transform processor.
func NewTransformProcessor(name string, transform TransformFunc) *TransformProcessor {
	return &TransformProcessor{name: name, transform: transform}
}

// Name returns the processor name.
func (t *TransformProcessor) Name() string {
	return t.name
}

// Process transforms events.
func (t *TransformProcessor) Process(event *profiler.ProfileEvent) (*profiler.ProfileEvent, error) {
	return t.transform(event), nil
}

// BaseSink provides common functionality for sink implementations.
type BaseSink struct {
	name   string
	health HealthStatus
}

// NewBaseSink creates a new base sink.
func NewBaseSink(name string) *BaseSink {
	return &BaseSink{
		name:   name,
		health: HealthOK,
	}
}

// Name returns the sink name.
func (b *BaseSink) Name() string {
	return b.name
}

// Health returns the current health status.
func (b *BaseSink) Health() HealthStatus {
	return b.health
}

// SetHealth sets the health status.
func (b *BaseSink) SetHealth(health HealthStatus) {
	b.health = health
}
