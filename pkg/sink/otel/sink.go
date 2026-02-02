// Package otel provides an OpenTelemetry sink for exporting profiling data.
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
)

// Sink exports profiling events to OpenTelemetry collectors.
type Sink struct {
	*sink.BaseSink

	config         *config.OTELSinkConfig
	logger         *zap.Logger
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracer         trace.Tracer
	meter          metric.Meter

	// Metric instruments
	workflowDuration   metric.Float64Histogram
	activityDuration   metric.Float64Histogram
	taskLatency        metric.Float64Histogram
	workflowCounter    metric.Int64Counter
	activityCounter    metric.Int64Counter
	errorCounter       metric.Int64Counter
	eventsProcessed    metric.Int64Counter
	scheduleToStartDur metric.Float64Histogram
}

// NewSink creates a new OTEL sink.
func NewSink(cfg *config.OTELSinkConfig, logger *zap.Logger) *Sink {
	return &Sink{
		BaseSink: sink.NewBaseSink("otel"),
		config:   cfg,
		logger:   logger,
	}
}

// Init initializes the OTEL sink.
func (s *Sink) Init(ctx context.Context) error {
	// Build resource attributes
	attrs := []attribute.KeyValue{
		semconv.ServiceName("temporal-profiler"),
	}
	for k, v := range s.config.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Create gRPC connection options
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	// Initialize trace exporter if enabled
	if s.config.Traces.Enabled {
		traceExporter, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(s.config.Endpoint),
			otlptracegrpc.WithDialOption(grpcOpts...),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return fmt.Errorf("failed to create trace exporter: %w", err)
		}

		s.tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)

		otel.SetTracerProvider(s.tracerProvider)
		s.tracer = s.tracerProvider.Tracer("temporal-profiler")
	}

	// Initialize metric exporter if enabled
	if s.config.Metrics.Enabled {
		metricExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(s.config.Endpoint),
			otlpmetricgrpc.WithDialOption(grpcOpts...),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return fmt.Errorf("failed to create metric exporter: %w", err)
		}

		s.meterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
				sdkmetric.WithInterval(s.config.Metrics.ExportInterval),
			)),
			sdkmetric.WithResource(res),
		)

		otel.SetMeterProvider(s.meterProvider)
		s.meter = s.meterProvider.Meter("temporal-profiler")

		// Initialize metric instruments
		if err := s.initMetrics(); err != nil {
			return fmt.Errorf("failed to initialize metrics: %w", err)
		}
	}

	s.logger.Info("OTEL sink initialized",
		zap.String("endpoint", s.config.Endpoint),
		zap.Bool("traces", s.config.Traces.Enabled),
		zap.Bool("metrics", s.config.Metrics.Enabled),
	)

	return nil
}

// initMetrics initializes all metric instruments.
func (s *Sink) initMetrics() error {
	var err error

	// Duration histograms
	s.workflowDuration, err = s.meter.Float64Histogram(
		"temporal.profiler.workflow.duration",
		metric.WithDescription("Duration of workflow executions"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	s.activityDuration, err = s.meter.Float64Histogram(
		"temporal.profiler.activity.duration",
		metric.WithDescription("Duration of activity executions"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	s.taskLatency, err = s.meter.Float64Histogram(
		"temporal.profiler.task.latency",
		metric.WithDescription("Latency of task processing"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	s.scheduleToStartDur, err = s.meter.Float64Histogram(
		"temporal.profiler.schedule_to_start.duration",
		metric.WithDescription("Time from scheduling to start"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	// Counters
	s.workflowCounter, err = s.meter.Int64Counter(
		"temporal.profiler.workflow.count",
		metric.WithDescription("Count of workflow events"),
	)
	if err != nil {
		return err
	}

	s.activityCounter, err = s.meter.Int64Counter(
		"temporal.profiler.activity.count",
		metric.WithDescription("Count of activity events"),
	)
	if err != nil {
		return err
	}

	s.errorCounter, err = s.meter.Int64Counter(
		"temporal.profiler.errors",
		metric.WithDescription("Count of errors"),
	)
	if err != nil {
		return err
	}

	s.eventsProcessed, err = s.meter.Int64Counter(
		"temporal.profiler.events.processed",
		metric.WithDescription("Total events processed"),
	)
	if err != nil {
		return err
	}

	return nil
}

// Write writes events to the OTEL collector.
func (s *Sink) Write(ctx context.Context, events []*profiler.ProfileEvent) error {
	for _, event := range events {
		// Record trace span if traces are enabled
		if s.config.Traces.Enabled && s.tracer != nil {
			s.recordTrace(ctx, event)
		}

		// Record metrics if metrics are enabled
		if s.config.Metrics.Enabled && s.meter != nil {
			s.recordMetrics(ctx, event)
		}
	}

	return nil
}

// recordTrace creates a trace span for the event.
func (s *Sink) recordTrace(ctx context.Context, event *profiler.ProfileEvent) {
	attrs := []attribute.KeyValue{
		attribute.String("temporal.workflow_id", event.WorkflowID),
		attribute.String("temporal.run_id", event.RunID),
		attribute.String("temporal.workflow_type", event.WorkflowType),
		attribute.String("temporal.namespace", event.Namespace),
		attribute.String("temporal.task_queue", event.TaskQueue),
		attribute.String("temporal.event_type", event.EventType.String()),
		attribute.String("temporal.status", event.Status.String()),
	}

	if event.ActivityID != "" {
		attrs = append(attrs, attribute.String("temporal.activity_id", event.ActivityID))
	}
	if event.ActivityType != "" {
		attrs = append(attrs, attribute.String("temporal.activity_type", event.ActivityType))
	}
	if event.ErrorMessage != "" {
		attrs = append(attrs, attribute.String("temporal.error_message", event.ErrorMessage))
	}
	if event.Attempt > 0 {
		attrs = append(attrs, attribute.Int("temporal.attempt", int(event.Attempt)))
	}

	// Create span with the event's timestamp
	_, span := s.tracer.Start(ctx, profiler.ExtractMethodName(event.OperationType),
		trace.WithTimestamp(event.Timestamp),
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindServer),
	)

	// Set status based on event status
	if event.IsError() {
		span.SetStatus(1, event.ErrorMessage) // Error status
	}

	// End span with calculated end time
	span.End(trace.WithTimestamp(event.Timestamp.Add(event.Duration)))
}

// recordMetrics records metrics for the event.
func (s *Sink) recordMetrics(ctx context.Context, event *profiler.ProfileEvent) {
	attrs := metric.WithAttributes(
		attribute.String("workflow_type", event.WorkflowType),
		attribute.String("namespace", event.Namespace),
		attribute.String("task_queue", event.TaskQueue),
		attribute.String("status", event.Status.String()),
		attribute.String("event_type", event.EventType.String()),
	)

	// Record event count
	s.eventsProcessed.Add(ctx, 1, attrs)

	// Record duration histogram based on event type
	durationSec := event.Duration.Seconds()

	if event.IsWorkflowEvent() {
		s.workflowDuration.Record(ctx, durationSec, attrs)
		s.workflowCounter.Add(ctx, 1, attrs)
	}

	if event.IsActivityEvent() {
		activityAttrs := metric.WithAttributes(
			attribute.String("workflow_type", event.WorkflowType),
			attribute.String("activity_type", event.ActivityType),
			attribute.String("namespace", event.Namespace),
			attribute.String("task_queue", event.TaskQueue),
			attribute.String("status", event.Status.String()),
		)
		s.activityDuration.Record(ctx, durationSec, activityAttrs)
		s.activityCounter.Add(ctx, 1, activityAttrs)
	}

	// Record task latency
	s.taskLatency.Record(ctx, durationSec, attrs)

	// Record schedule-to-start if available
	if event.ScheduleToStartDuration > 0 {
		s.scheduleToStartDur.Record(ctx, event.ScheduleToStartDuration.Seconds(), attrs)
	}

	// Record errors
	if event.IsError() {
		errorAttrs := metric.WithAttributes(
			attribute.String("workflow_type", event.WorkflowType),
			attribute.String("namespace", event.Namespace),
			attribute.String("error_type", event.ErrorType),
			attribute.String("event_type", event.EventType.String()),
		)
		s.errorCounter.Add(ctx, 1, errorAttrs)
	}
}

// Flush flushes any pending data.
func (s *Sink) Flush(ctx context.Context) error {
	if s.tracerProvider != nil {
		if err := s.tracerProvider.ForceFlush(ctx); err != nil {
			s.logger.Warn("failed to flush traces", zap.Error(err))
		}
	}
	if s.meterProvider != nil {
		if err := s.meterProvider.ForceFlush(ctx); err != nil {
			s.logger.Warn("failed to flush metrics", zap.Error(err))
		}
	}
	return nil
}

// Close shuts down the sink.
func (s *Sink) Close(ctx context.Context) error {
	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(ctx); err != nil {
			s.logger.Error("failed to shutdown trace provider", zap.Error(err))
		}
	}
	if s.meterProvider != nil {
		if err := s.meterProvider.Shutdown(ctx); err != nil {
			s.logger.Error("failed to shutdown meter provider", zap.Error(err))
		}
	}
	return nil
}

// Helper to extract just method name
func init() {
	// Register global error handler
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		// Silently ignore OTEL errors to avoid log spam
	}))
}
