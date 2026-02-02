package config

import "time"

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			ListenAddr:   ":7234",
			UpstreamAddr: "localhost:7233",
			TLS: TLSConfig{
				Enabled: false,
			},
		},
		Profiler: ProfilerConfig{
			BufferSize:    10000,
			BatchSize:     100,
			FlushInterval: 5 * time.Second,
			WorkerCount:   2,
		},
		Sampling: SamplingConfig{
			Enabled:  false,
			Rate:     1.0,
			Strategy: "probabilistic",
		},
		Visibility: VisibilityConfig{
			Enabled:      false,
			PollInterval: 30 * time.Second,
			CacheTTL:     5 * time.Minute,
		},
		Sinks: SinksConfig{
			OTEL: &OTELSinkConfig{
				Enabled:  false,
				Endpoint: "localhost:4317",
				Protocol: "grpc",
				ResourceAttributes: map[string]string{
					"service.name": "temporal-profiler",
				},
				Metrics: OTELMetricsConfig{
					Enabled:        true,
					ExportInterval: 10 * time.Second,
				},
				Traces: OTELTracesConfig{
					Enabled: true,
					Sampler: "always_on",
				},
			},
			Slack: &SlackSinkConfig{
				Enabled: false,
				RateLimit: RateLimitConfig{
					MaxPerMinute: 10,
					Cooldown:     5 * time.Minute,
				},
			},
			File: &FileSinkConfig{
				Enabled: false,
				Path:    "/var/log/temporal-profiler/events.jsonl",
				Rotation: FileRotationConfig{
					MaxSize:    "100MB",
					MaxAge:     "7d",
					MaxBackups: 5,
				},
			},
			Webhook: &WebhookSinkConfig{
				Enabled:   false,
				Method:    "POST",
				BatchSize: 50,
				Timeout:   10 * time.Second,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Admin: AdminConfig{
			Enabled: true,
			Addr:    ":8080",
			Endpoints: EndpointsConfig{
				Health:  "/health",
				Metrics: "/metrics",
				Ready:   "/ready",
			},
		},
	}
}
