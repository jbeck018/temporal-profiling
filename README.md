# Temporal Profiler

A **runtime-agnostic profiling proxy** for [Temporal.io](https://temporal.io) that captures workflow and activity metrics with **zero overhead** and exports them to OpenTelemetry, Slack, and other destinations.

## Features

- **Runtime Agnostic**: Works with ALL Temporal SDKs (Go, TypeScript, Python, Java, .NET, PHP) without any code changes
- **Zero Overhead**: Lock-free ring buffer and async processing ensure minimal impact on your application
- **OpenTelemetry Native**: Export traces and metrics to any OTEL-compatible backend (Jaeger, Prometheus, Datadog, etc.)
- **Slack Alerts**: Get notified about failed workflows, slow activities, and custom conditions
- **Easy Setup**: Just change one address in your SDK configuration
- **Pluggable Sinks**: Built-in support for OTEL, Slack, File, and Webhook outputs

## Quick Start

### 1. Run the Profiler

```bash
# Using Docker
docker run -p 7234:7234 -p 8080:8080 \
  -e TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=your-temporal-server:7233 \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENABLED=true \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT=your-otel-collector:4317 \
  ghcr.io/temporal-profiling/temporal-profiler:latest

# Or build from source
go build -o temporal-profiler ./cmd/temporal-profiler
./temporal-profiler start --upstream your-temporal-server:7233
```

### 2. Point Your SDK to the Profiler

Change your Temporal SDK connection from the server address to the profiler address:

**Go SDK:**
```go
// Before
client.Dial(client.Options{HostPort: "localhost:7233"})

// After - just change the port!
client.Dial(client.Options{HostPort: "localhost:7234"})
```

**TypeScript SDK:**
```typescript
// Before
await Connection.connect({ address: 'localhost:7233' });

// After
await Connection.connect({ address: 'localhost:7234' });
```

**Python SDK:**
```python
# Before
await Client.connect("localhost:7233")

# After
await Client.connect("localhost:7234")
```

That's it! The profiler transparently proxies all traffic while collecting profiling data.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Your Application                             │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐       │
│  │ Go SDK  │ │ TS SDK  │ │ Python  │ │ Java   │ │ .NET    │       │
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬───┘ └────┬────┘       │
│       │           │           │           │          │             │
│       └───────────┴───────────┴─────┬─────┴──────────┘             │
│                                     │ gRPC                          │
│                                     ▼                               │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                   TEMPORAL PROFILER PROXY                     │  │
│  │  ┌───────────┐  ┌──────────┐  ┌────────────┐  ┌───────────┐  │  │
│  │  │  gRPC     │  │ Ring     │  │  Sink      │  │  OTEL     │  │  │
│  │  │ Intercept │─▶│ Buffer   │─▶│  Pipeline  │─▶│  Slack    │  │  │
│  │  └───────────┘  └──────────┘  └────────────┘  │  File     │  │  │
│  │        │                                       │  Webhook  │  │  │
│  │        │ Forward                               └───────────┘  │  │
│  │        ▼                                                      │  │
│  │  ┌───────────┐                                                │  │
│  │  │ Temporal  │                                                │  │
│  │  │ Server    │                                                │  │
│  │  └───────────┘                                                │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Configuration

Create a `temporal-profiler.yaml` file or use environment variables:

```yaml
# Proxy settings
proxy:
  listen_addr: ":7234"           # Where SDKs connect
  upstream_addr: "localhost:7233" # Actual Temporal server

# Profiler settings
profiler:
  buffer_size: 10000             # Ring buffer capacity
  batch_size: 100                # Events per batch
  flush_interval: "5s"           # Max time before flush
  worker_count: 2                # Background workers

# Output sinks
sinks:
  # OpenTelemetry
  otel:
    enabled: true
    endpoint: "localhost:4317"
    metrics:
      enabled: true
      export_interval: "10s"
    traces:
      enabled: true

  # Slack notifications
  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
    alerts:
      - name: "workflow_failed"
        condition: "event_type == WORKFLOW_FAILED"
        severity: "error"
      - name: "slow_workflow"
        condition: "workflow_duration > 30s"
        severity: "warning"

  # File output (JSONL format)
  file:
    enabled: false
    path: "/var/log/temporal-profiler/events.jsonl"

  # Generic webhook
  webhook:
    enabled: false
    url: "https://your-endpoint.com/events"

# Admin server
admin:
  enabled: true
  addr: ":8080"
```

### Environment Variables

All config options can be set via environment variables with the `TEMPORAL_PROFILER_` prefix:

```bash
TEMPORAL_PROFILER_PROXY_LISTEN_ADDR=":7234"
TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR="temporal:7233"
TEMPORAL_PROFILER_SINKS_OTEL_ENABLED="true"
TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT="otel-collector:4317"
TEMPORAL_PROFILER_SINKS_SLACK_WEBHOOK_URL="https://hooks.slack.com/..."
```

## Full Stack Example (Docker Compose)

Start a complete observability stack with one command:

```bash
cd deploy/docker
docker-compose up -d
```

This starts:
- **Temporal Server** (port 7233)
- **Temporal Profiler** (port 7234 - proxy, 8081 - admin)
- **OpenTelemetry Collector** (port 4317)
- **Jaeger** (port 16686 - UI)
- **Prometheus** (port 9090)
- **Grafana** (port 3000)

Then point your SDKs to `localhost:7234` and view:
- Traces in Jaeger: http://localhost:16686
- Metrics in Prometheus: http://localhost:9090
- Dashboards in Grafana: http://localhost:3000

## Metrics Exported

The profiler exports the following metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `temporal.profiler.workflow.duration` | Histogram | Duration of workflow executions |
| `temporal.profiler.activity.duration` | Histogram | Duration of activity executions |
| `temporal.profiler.task.latency` | Histogram | Latency of task processing |
| `temporal.profiler.workflow.count` | Counter | Count of workflow events |
| `temporal.profiler.activity.count` | Counter | Count of activity events |
| `temporal.profiler.errors` | Counter | Count of errors |
| `temporal.profiler.events.processed` | Counter | Total events processed |
| `temporal.profiler.schedule_to_start.duration` | Histogram | Schedule-to-start latency |

All metrics include labels for `workflow_type`, `activity_type`, `namespace`, `task_queue`, and `status`.

## Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| gRPC Proxy | 7234 | SDKs connect here |
| `/health` | 8080 | Health check |
| `/ready` | 8080 | Readiness check |
| `/metrics` | 8080 | Prometheus metrics |
| `/stats` | 8080 | JSON statistics |

## Zero Overhead Design

The profiler is designed for production use with minimal overhead:

1. **Lock-free Ring Buffer**: Events are pushed to a pre-allocated ring buffer using atomic operations - no locks in the hot path
2. **Non-blocking Recording**: If the buffer is full, events are dropped rather than blocking the request
3. **Async Processing**: Background workers process events and send to sinks without affecting request latency
4. **Batch Processing**: Events are sent to sinks in batches to amortize overhead

Typical overhead is **< 100μs** per request.

## Development

```bash
# Build
make build

# Run tests
make test

# Run with hot reload
make dev

# Generate default config
./temporal-profiler config > temporal-profiler.yaml
```

## License

MIT License - see [LICENSE](LICENSE) for details.
