package sink

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/temporal-profiling/temporal-profiler/pkg/buffer"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
)

// PipelineConfig configures the sink pipeline.
type PipelineConfig struct {
	BatchSize     int
	FlushInterval time.Duration
	WorkerCount   int
}

// Pipeline manages multiple sinks and routes events to them.
type Pipeline struct {
	sinks      []Sink
	processors []Processor
	buffer     buffer.Buffer
	config     PipelineConfig
	logger     *zap.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	eventsProcessed int64
	eventsDropped   int64
	batchesSent     int64
	errors          int64
}

// NewPipeline creates a new sink pipeline.
func NewPipeline(buf buffer.Buffer, config PipelineConfig, logger *zap.Logger) *Pipeline {
	return &Pipeline{
		sinks:      make([]Sink, 0),
		processors: make([]Processor, 0),
		buffer:     buf,
		config:     config,
		logger:     logger,
	}
}

// AddSink adds a sink to the pipeline.
func (p *Pipeline) AddSink(sink Sink) {
	p.sinks = append(p.sinks, sink)
}

// AddProcessor adds a processor to the pipeline.
func (p *Pipeline) AddProcessor(processor Processor) {
	p.processors = append(p.processors, processor)
}

// Start starts the pipeline workers.
func (p *Pipeline) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Initialize all sinks
	for _, sink := range p.sinks {
		if err := sink.Init(p.ctx); err != nil {
			return err
		}
		p.logger.Info("initialized sink", zap.String("sink", sink.Name()))
	}

	// Start worker goroutines
	for i := 0; i < p.config.WorkerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	p.logger.Info("pipeline started",
		zap.Int("workers", p.config.WorkerCount),
		zap.Int("sinks", len(p.sinks)),
	)

	return nil
}

// Stop gracefully stops the pipeline.
func (p *Pipeline) Stop(ctx context.Context) error {
	p.cancel()
	p.wg.Wait()

	// Flush and close all sinks
	for _, sink := range p.sinks {
		if err := sink.Flush(ctx); err != nil {
			p.logger.Error("error flushing sink", zap.String("sink", sink.Name()), zap.Error(err))
		}
		if err := sink.Close(ctx); err != nil {
			p.logger.Error("error closing sink", zap.String("sink", sink.Name()), zap.Error(err))
		}
	}

	p.logger.Info("pipeline stopped",
		zap.Int64("events_processed", p.eventsProcessed),
		zap.Int64("batches_sent", p.batchesSent),
	)

	return nil
}

// worker is the background worker that processes events.
func (p *Pipeline) worker(id int) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*profiler.ProfileEvent, 0, p.config.BatchSize)

	for {
		select {
		case <-p.ctx.Done():
			// Final flush
			if len(batch) > 0 {
				p.flush(batch)
			}
			return

		case <-ticker.C:
			// Flush on interval
			if len(batch) > 0 {
				p.flush(batch)
				batch = batch[:0]
			}

		default:
			// Consume events from buffer
			events := p.buffer.Consume(p.config.BatchSize - len(batch))
			if len(events) == 0 {
				// No events, sleep briefly to avoid busy loop
				time.Sleep(10 * time.Millisecond)
				continue
			}

			// Process events
			for _, event := range events {
				processed := p.processEvent(event)
				if processed != nil {
					batch = append(batch, processed)
				}
			}

			// Flush if batch is full
			if len(batch) >= p.config.BatchSize {
				p.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

// processEvent runs the event through all processors.
func (p *Pipeline) processEvent(event *profiler.ProfileEvent) *profiler.ProfileEvent {
	current := event
	for _, processor := range p.processors {
		var err error
		current, err = processor.Process(current)
		if err != nil {
			p.logger.Warn("processor error",
				zap.String("processor", processor.Name()),
				zap.Error(err),
			)
			return nil
		}
		if current == nil {
			return nil // Filtered out
		}
	}
	return current
}

// flush sends a batch of events to all sinks.
func (p *Pipeline) flush(events []*profiler.ProfileEvent) {
	if len(events) == 0 {
		return
	}

	p.eventsProcessed += int64(len(events))
	p.batchesSent++

	// Send to all sinks in parallel
	var wg sync.WaitGroup
	for _, sink := range p.sinks {
		if sink.Health() == HealthUnhealthy {
			continue // Skip unhealthy sinks
		}

		wg.Add(1)
		go func(s Sink) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
			defer cancel()

			if err := s.Write(ctx, events); err != nil {
				p.errors++
				p.logger.Error("sink write error",
					zap.String("sink", s.Name()),
					zap.Error(err),
				)
			}
		}(sink)
	}
	wg.Wait()
}

// Stats returns pipeline statistics.
type Stats struct {
	EventsProcessed int64
	EventsDropped   int64
	BatchesSent     int64
	Errors          int64
	BufferStats     buffer.Stats
	SinkHealth      map[string]HealthStatus
}

// Stats returns current pipeline statistics.
func (p *Pipeline) Stats() Stats {
	sinkHealth := make(map[string]HealthStatus)
	for _, sink := range p.sinks {
		sinkHealth[sink.Name()] = sink.Health()
	}

	return Stats{
		EventsProcessed: p.eventsProcessed,
		EventsDropped:   p.eventsDropped,
		BatchesSent:     p.batchesSent,
		Errors:          p.errors,
		BufferStats:     p.buffer.Stats(),
		SinkHealth:      sinkHealth,
	}
}
