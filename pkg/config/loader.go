package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Loader handles loading configuration from multiple sources.
type Loader struct {
	v *viper.Viper
}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	v := viper.New()

	// Set config file name and paths
	v.SetConfigName("temporal-profiler")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/temporal-profiler")
	v.AddConfigPath("$HOME/.temporal-profiler")

	// Environment variables
	v.SetEnvPrefix("TEMPORAL_PROFILER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	return &Loader{v: v}
}

// Load loads configuration from all sources.
func (l *Loader) Load() (*Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// Try to read config file (optional)
	if err := l.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, we use defaults + env vars
	}

	// Unmarshal into config struct
	if err := l.v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Expand environment variables in sensitive fields
	l.expandEnvVars(cfg)

	return cfg, nil
}

// LoadFromFile loads configuration from a specific file path.
func (l *Loader) LoadFromFile(path string) (*Config, error) {
	l.v.SetConfigFile(path)
	return l.Load()
}

// expandEnvVars expands environment variables in sensitive config fields.
func (l *Loader) expandEnvVars(cfg *Config) {
	if cfg.Sinks.Slack != nil {
		cfg.Sinks.Slack.WebhookURL = os.ExpandEnv(cfg.Sinks.Slack.WebhookURL)
	}

	if cfg.Sinks.OTEL != nil {
		for k, v := range cfg.Sinks.OTEL.Headers {
			cfg.Sinks.OTEL.Headers[k] = os.ExpandEnv(v)
		}
	}

	if cfg.Sinks.Webhook != nil {
		cfg.Sinks.Webhook.URL = os.ExpandEnv(cfg.Sinks.Webhook.URL)
		for k, v := range cfg.Sinks.Webhook.Headers {
			cfg.Sinks.Webhook.Headers[k] = os.ExpandEnv(v)
		}
	}
}

// Validate validates the configuration.
func Validate(cfg *Config) error {
	if cfg.Proxy.ListenAddr == "" {
		return fmt.Errorf("proxy.listen_addr is required")
	}
	if cfg.Proxy.UpstreamAddr == "" {
		return fmt.Errorf("proxy.upstream_addr is required")
	}
	if cfg.Profiler.BufferSize <= 0 {
		return fmt.Errorf("profiler.buffer_size must be positive")
	}
	if cfg.Profiler.BatchSize <= 0 {
		return fmt.Errorf("profiler.batch_size must be positive")
	}
	if cfg.Profiler.WorkerCount <= 0 {
		return fmt.Errorf("profiler.worker_count must be positive")
	}
	if cfg.Sampling.Enabled && (cfg.Sampling.Rate < 0 || cfg.Sampling.Rate > 1) {
		return fmt.Errorf("sampling.rate must be between 0 and 1")
	}

	// Validate OTEL config if enabled
	if cfg.Sinks.OTEL != nil && cfg.Sinks.OTEL.Enabled {
		if cfg.Sinks.OTEL.Endpoint == "" {
			return fmt.Errorf("sinks.otel.endpoint is required when OTEL is enabled")
		}
	}

	// Validate Slack config if enabled
	if cfg.Sinks.Slack != nil && cfg.Sinks.Slack.Enabled {
		if cfg.Sinks.Slack.WebhookURL == "" {
			return fmt.Errorf("sinks.slack.webhook_url is required when Slack is enabled")
		}
	}

	return nil
}
