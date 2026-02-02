// Package config provides configuration types and loading for temporal-profiler.
package config

import (
	"time"
)

// Config is the root configuration structure.
type Config struct {
	Proxy      ProxyConfig      `mapstructure:"proxy"`
	Profiler   ProfilerConfig   `mapstructure:"profiler"`
	Sinks      SinksConfig      `mapstructure:"sinks"`
	Sampling   SamplingConfig   `mapstructure:"sampling"`
	Visibility VisibilityConfig `mapstructure:"visibility"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Admin      AdminConfig      `mapstructure:"admin"`
}

// ProxyConfig configures the gRPC proxy server.
type ProxyConfig struct {
	ListenAddr   string    `mapstructure:"listen_addr"`
	UpstreamAddr string    `mapstructure:"upstream_addr"`
	TLS          TLSConfig `mapstructure:"tls"`
}

// TLSConfig configures TLS settings.
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
}

// ProfilerConfig configures the profiling engine.
type ProfilerConfig struct {
	BufferSize    int           `mapstructure:"buffer_size"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	WorkerCount   int           `mapstructure:"worker_count"`
}

// SamplingConfig configures event sampling.
type SamplingConfig struct {
	Enabled  bool    `mapstructure:"enabled"`
	Rate     float64 `mapstructure:"rate"`
	Strategy string  `mapstructure:"strategy"`
}

// VisibilityConfig configures optional Temporal Visibility API enrichment.
type VisibilityConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	CacheTTL     time.Duration `mapstructure:"cache_ttl"`
}

// SinksConfig configures all output sinks.
type SinksConfig struct {
	OTEL    *OTELSinkConfig    `mapstructure:"otel"`
	Slack   *SlackSinkConfig   `mapstructure:"slack"`
	File    *FileSinkConfig    `mapstructure:"file"`
	Webhook *WebhookSinkConfig `mapstructure:"webhook"`
}

// OTELSinkConfig configures the OpenTelemetry sink.
type OTELSinkConfig struct {
	Enabled            bool              `mapstructure:"enabled"`
	Endpoint           string            `mapstructure:"endpoint"`
	Protocol           string            `mapstructure:"protocol"`
	Headers            map[string]string `mapstructure:"headers"`
	TLS                TLSConfig         `mapstructure:"tls"`
	ResourceAttributes map[string]string `mapstructure:"resource_attributes"`
	Metrics            OTELMetricsConfig `mapstructure:"metrics"`
	Traces             OTELTracesConfig  `mapstructure:"traces"`
}

// OTELMetricsConfig configures OTEL metrics export.
type OTELMetricsConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	ExportInterval time.Duration `mapstructure:"export_interval"`
}

// OTELTracesConfig configures OTEL trace export.
type OTELTracesConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Sampler string `mapstructure:"sampler"`
}

// SlackSinkConfig configures the Slack notification sink.
type SlackSinkConfig struct {
	Enabled    bool              `mapstructure:"enabled"`
	WebhookURL string            `mapstructure:"webhook_url"`
	Channel    string            `mapstructure:"channel"`
	Alerts     []AlertRuleConfig `mapstructure:"alerts"`
	RateLimit  RateLimitConfig   `mapstructure:"rate_limit"`
}

// AlertRuleConfig configures a single alert rule.
type AlertRuleConfig struct {
	Name      string `mapstructure:"name"`
	Condition string `mapstructure:"condition"`
	Severity  string `mapstructure:"severity"`
}

// RateLimitConfig configures rate limiting for alerts.
type RateLimitConfig struct {
	MaxPerMinute int           `mapstructure:"max_per_minute"`
	Cooldown     time.Duration `mapstructure:"cooldown"`
}

// FileSinkConfig configures the file output sink.
type FileSinkConfig struct {
	Enabled  bool               `mapstructure:"enabled"`
	Path     string             `mapstructure:"path"`
	Rotation FileRotationConfig `mapstructure:"rotation"`
}

// FileRotationConfig configures log file rotation.
type FileRotationConfig struct {
	MaxSize    string `mapstructure:"max_size"`
	MaxAge     string `mapstructure:"max_age"`
	MaxBackups int    `mapstructure:"max_backups"`
}

// WebhookSinkConfig configures a generic webhook sink.
type WebhookSinkConfig struct {
	Enabled   bool              `mapstructure:"enabled"`
	URL       string            `mapstructure:"url"`
	Method    string            `mapstructure:"method"`
	Headers   map[string]string `mapstructure:"headers"`
	BatchSize int               `mapstructure:"batch_size"`
	Timeout   time.Duration     `mapstructure:"timeout"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// AdminConfig configures the admin server.
type AdminConfig struct {
	Enabled   bool            `mapstructure:"enabled"`
	Addr      string          `mapstructure:"addr"`
	Endpoints EndpointsConfig `mapstructure:"endpoints"`
}

// EndpointsConfig configures admin endpoints.
type EndpointsConfig struct {
	Health  string `mapstructure:"health"`
	Metrics string `mapstructure:"metrics"`
	Ready   string `mapstructure:"ready"`
}
