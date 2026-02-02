// Package main is the entry point for the temporal-profiler CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/temporal-profiling/temporal-profiler/pkg/admin"
	"github.com/temporal-profiling/temporal-profiler/pkg/buffer"
	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/proxy"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink/file"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink/otel"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink/slack"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink/webhook"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var (
	cfgFile      string
	listenAddr   string
	upstreamAddr string
	logLevel     string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "temporal-profiler",
		Short: "A runtime-agnostic profiling proxy for Temporal.io",
		Long: `Temporal Profiler is a gRPC proxy that intercepts Temporal SDK traffic
to collect profiling data with zero overhead. It exports metrics and traces
to OpenTelemetry collectors, Slack, and other destinations.

Simply point your Temporal SDKs to the profiler's address instead of the
Temporal server, and the profiler will transparently forward all traffic
while collecting profiling data.`,
	}

	// Start command
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the profiler proxy",
		RunE:  runStart,
	}

	startCmd.Flags().StringVarP(&cfgFile, "config", "c", "", "Config file path")
	startCmd.Flags().StringVarP(&listenAddr, "listen", "l", ":7234", "Address to listen on")
	startCmd.Flags().StringVarP(&upstreamAddr, "upstream", "u", "localhost:7233", "Upstream Temporal server address")
	startCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("temporal-profiler %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", buildDate)
		},
	}

	// Config command
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Print default configuration",
		Run: func(cmd *cobra.Command, args []string) {
			printDefaultConfig()
		},
	}

	rootCmd.AddCommand(startCmd, versionCmd, configCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	// Setup logger
	logger, err := setupLogger(logLevel)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting temporal-profiler",
		zap.String("version", version),
		zap.String("listen", listenAddr),
		zap.String("upstream", upstreamAddr),
	)

	// Load configuration
	loader := config.NewLoader()
	var cfg *config.Config
	if cfgFile != "" {
		cfg, err = loader.LoadFromFile(cfgFile)
	} else {
		cfg, err = loader.Load()
	}
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI overrides
	if listenAddr != "" {
		cfg.Proxy.ListenAddr = listenAddr
	}
	if upstreamAddr != "" {
		cfg.Proxy.UpstreamAddr = upstreamAddr
	}

	// Validate config
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create ring buffer
	buf := buffer.NewRingBuffer(cfg.Profiler.BufferSize)
	logger.Info("created ring buffer", zap.Int("capacity", buf.Cap()))

	// Create sink pipeline
	pipeline := sink.NewPipeline(buf, sink.PipelineConfig{
		BatchSize:     cfg.Profiler.BatchSize,
		FlushInterval: cfg.Profiler.FlushInterval,
		WorkerCount:   cfg.Profiler.WorkerCount,
	}, logger)

	// Add configured sinks
	if err := addSinks(pipeline, cfg, logger); err != nil {
		return fmt.Errorf("failed to add sinks: %w", err)
	}

	// Start pipeline
	if err := pipeline.Start(ctx); err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	// Create and start proxy server
	proxyServer, err := proxy.NewServer(cfg.Proxy, buf, logger)
	if err != nil {
		return fmt.Errorf("failed to create proxy server: %w", err)
	}

	if err := proxyServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start proxy server: %w", err)
	}

	// Create and start admin server
	var adminServer *admin.Server
	if cfg.Admin.Enabled {
		adminServer = admin.NewServer(cfg.Admin, logger)
		adminServer.SetPipeline(pipeline)
		if err := adminServer.Start(ctx); err != nil {
			return fmt.Errorf("failed to start admin server: %w", err)
		}
		adminServer.SetReady(true)
	}

	logger.Info("temporal-profiler started",
		zap.String("proxy", proxyServer.Addr()),
		zap.String("admin", cfg.Admin.Addr),
	)

	// Wait for shutdown signal
	sig := <-sigCh
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if adminServer != nil {
		adminServer.SetReady(false)
		adminServer.Stop(shutdownCtx)
	}

	if err := proxyServer.Stop(shutdownCtx); err != nil {
		logger.Error("error stopping proxy server", zap.Error(err))
	}

	if err := pipeline.Stop(shutdownCtx); err != nil {
		logger.Error("error stopping pipeline", zap.Error(err))
	}

	logger.Info("temporal-profiler stopped")
	return nil
}

func addSinks(pipeline *sink.Pipeline, cfg *config.Config, logger *zap.Logger) error {
	// Add OTEL sink if enabled
	if cfg.Sinks.OTEL != nil && cfg.Sinks.OTEL.Enabled {
		otelSink := otel.NewSink(cfg.Sinks.OTEL, logger)
		pipeline.AddSink(otelSink)
		logger.Info("added OTEL sink", zap.String("endpoint", cfg.Sinks.OTEL.Endpoint))
	}

	// Add Slack sink if enabled
	if cfg.Sinks.Slack != nil && cfg.Sinks.Slack.Enabled {
		slackSink := slack.NewSink(cfg.Sinks.Slack, logger)
		pipeline.AddSink(slackSink)
		logger.Info("added Slack sink")
	}

	// Add File sink if enabled
	if cfg.Sinks.File != nil && cfg.Sinks.File.Enabled {
		fileSink := file.NewSink(cfg.Sinks.File, logger)
		pipeline.AddSink(fileSink)
		logger.Info("added File sink", zap.String("path", cfg.Sinks.File.Path))
	}

	// Add Webhook sink if enabled
	if cfg.Sinks.Webhook != nil && cfg.Sinks.Webhook.Enabled {
		webhookSink := webhook.NewSink(cfg.Sinks.Webhook, logger)
		pipeline.AddSink(webhookSink)
		logger.Info("added Webhook sink", zap.String("url", cfg.Sinks.Webhook.URL))
	}

	return nil
}

func setupLogger(level string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	cfg := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapLevel),
		Development: false,
		Encoding:    "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return cfg.Build()
}

func printDefaultConfig() {
	defaultConfig := `# Temporal Profiler Configuration

# Proxy settings
proxy:
  listen_addr: ":7234"           # Where SDKs connect
  upstream_addr: "localhost:7233" # Actual Temporal server
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
    ca_file: ""

# Profiler settings
profiler:
  buffer_size: 10000             # Ring buffer capacity
  batch_size: 100                # Events per batch to sinks
  flush_interval: "5s"           # Max time before flush
  worker_count: 2                # Background workers

# Sampling (for high-volume scenarios)
sampling:
  enabled: false
  rate: 1.0                      # 1.0 = 100%, 0.1 = 10%
  strategy: "probabilistic"      # or "rate_limiting", "adaptive"

# Output sinks
sinks:
  # OpenTelemetry
  otel:
    enabled: true
    endpoint: "localhost:4317"   # OTLP gRPC endpoint
    protocol: "grpc"
    resource_attributes:
      service.name: "temporal-profiler"
    metrics:
      enabled: true
      export_interval: "10s"
    traces:
      enabled: true
      sampler: "always_on"

  # Slack notifications
  slack:
    enabled: false
    webhook_url: "${SLACK_WEBHOOK_URL}"
    channel: "#temporal-alerts"
    alerts:
      - name: "workflow_failed"
        condition: "event_type == WORKFLOW_FAILED"
        severity: "error"
      - name: "slow_workflow"
        condition: "workflow_duration > 30s"
        severity: "warning"
    rate_limit:
      max_per_minute: 10
      cooldown: "5m"

  # File output
  file:
    enabled: false
    path: "/var/log/temporal-profiler/events.jsonl"
    rotation:
      max_size: "100MB"
      max_age: "7d"
      max_backups: 5

  # Generic webhook
  webhook:
    enabled: false
    url: "https://example.com/temporal-events"
    method: "POST"
    headers:
      Content-Type: "application/json"
    batch_size: 50
    timeout: "10s"

# Logging
logging:
  level: "info"
  format: "json"
  output: "stdout"

# Admin server
admin:
  enabled: true
  addr: ":8080"
  endpoints:
    health: "/health"
    metrics: "/metrics"
    ready: "/ready"
`
	fmt.Println(defaultConfig)
}
