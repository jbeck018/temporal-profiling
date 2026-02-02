# Temporal Profiler Deployment Guide

This guide covers deploying Temporal Profiler across different platforms and environments.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Docker](#docker)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Helm](#helm)
- [AWS ECS/Fargate](#aws-ecsfargate)
- [Google Cloud Run](#google-cloud-run)
- [Azure Container Apps](#azure-container-apps)
- [Configuration Reference](#configuration-reference)
- [SDK Integration](#sdk-integration)
- [TLS/mTLS Setup](#tlsmtls-setup)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Overview

Temporal Profiler is a gRPC proxy that sits between your Temporal SDKs and the Temporal server. It intercepts all traffic to collect profiling data with zero overhead.

**Architecture:**
```
┌─────────────┐      ┌───────────────────┐      ┌─────────────────┐
│   SDK App   │─────▶│ Temporal Profiler │─────▶│ Temporal Server │
│             │:7234 │     (Proxy)       │:7233 │                 │
└─────────────┘      └───────────────────┘      └─────────────────┘
                              │
                              ▼
                     ┌─────────────────┐
                     │  OTEL/Slack/etc │
                     └─────────────────┘
```

**Ports:**
- `7234` - gRPC proxy (SDKs connect here)
- `8080` - Admin server (health, metrics)

## Quick Start

### Docker (Simplest)

```bash
docker run -d \
  --name temporal-profiler \
  -p 7234:7234 \
  -p 8080:8080 \
  -e TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=temporal-server:7233 \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENABLED=true \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT=otel-collector:4317 \
  ghcr.io/temporal-profiling/temporal-profiler:latest
```

Then point your SDK to `localhost:7234` instead of `localhost:7233`.

## Docker

### Basic Usage

```bash
docker run -d \
  --name temporal-profiler \
  -p 7234:7234 \
  -p 8080:8080 \
  -e TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=your-temporal:7233 \
  ghcr.io/temporal-profiling/temporal-profiler:latest
```

### Production Configuration

```bash
docker run -d \
  --name temporal-profiler \
  -p 7234:7234 \
  -p 8080:8080 \
  # Mount config file
  -v /etc/temporal-profiler/config.yaml:/etc/temporal-profiler/temporal-profiler.yaml:ro \
  # Mount TLS certificates (if using TLS)
  -v /etc/temporal-profiler/certs:/certs:ro \
  # Environment configuration
  -e TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=temporal:7233 \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENABLED=true \
  -e TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT=otel-collector:4317 \
  -e TEMPORAL_PROFILER_SINKS_SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL}" \
  # Resource limits
  --memory=1g \
  --cpus=2.0 \
  # Health check
  --health-cmd="wget -q -O - http://localhost:8080/ready || exit 1" \
  --health-interval=10s \
  --health-timeout=5s \
  --health-retries=3 \
  # Security
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges:true \
  # Restart policy
  --restart unless-stopped \
  ghcr.io/temporal-profiling/temporal-profiler:latest
```

## Docker Compose

### Full Observability Stack

The `deploy/docker/docker-compose.yml` provides a complete local development setup:

```bash
cd deploy/docker
docker-compose up -d
```

This starts:
- **Temporal Server** (port 7233)
- **Temporal Profiler** (port 7234 - proxy, 8081 - admin)
- **Temporal UI** (port 8080)
- **OpenTelemetry Collector** (port 4317)
- **Jaeger** (port 16686 - UI)
- **Prometheus** (port 9090)
- **Grafana** (port 3000)

**Access Points:**
| Service | URL |
|---------|-----|
| Temporal Profiler (SDK connection) | `localhost:7234` |
| Temporal Profiler Admin | `http://localhost:8081` |
| Temporal UI | `http://localhost:8080` |
| Jaeger | `http://localhost:16686` |
| Prometheus | `http://localhost:9090` |
| Grafana | `http://localhost:3000` |

### Environment Customization

Create a `.env` file:

```bash
# .env
TEMPORAL_SERVER_ADDR=temporal:7233
OTEL_COLLECTOR_ENDPOINT=otel-collector:4317
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
LOG_LEVEL=info
BUFFER_SIZE=10000
```

## Kubernetes

### Quick Start with Kustomize

```bash
# Development
kubectl apply -k deploy/kubernetes/base

# Production
kubectl apply -k deploy/kubernetes/overlays/production
```

### Manual Deployment

```bash
# Create namespace
kubectl create namespace temporal-profiler

# Apply manifests
kubectl apply -f deploy/kubernetes/base/
```

### Verify Deployment

```bash
kubectl -n temporal-profiler get pods
kubectl -n temporal-profiler get svc
kubectl -n temporal-profiler logs -l app.kubernetes.io/name=temporal-profiler
```

### Production Considerations

1. **Replicas**: Minimum 2 for HA
2. **Pod Anti-Affinity**: Spreads pods across nodes
3. **Topology Spread**: Distributes across availability zones
4. **PodDisruptionBudget**: Prevents simultaneous evictions
5. **HorizontalPodAutoscaler**: Scales based on CPU/memory

## Helm

### Installation

```bash
# Add repository (when published)
helm repo add temporal-profiler https://temporal-profiling.github.io/charts
helm repo update

# Install with defaults
helm install temporal-profiler temporal-profiler/temporal-profiler \
  --namespace temporal-profiler \
  --create-namespace

# Install with custom values
helm install temporal-profiler temporal-profiler/temporal-profiler \
  --namespace temporal-profiler \
  --create-namespace \
  --set config.proxy.upstreamAddr="temporal.temporal.svc:7233" \
  --set config.sinks.otel.endpoint="otel-collector.monitoring.svc:4317" \
  --set config.sinks.slack.enabled=true

# Install from local chart
helm install temporal-profiler ./deploy/helm/temporal-profiler \
  --namespace temporal-profiler \
  --create-namespace \
  -f my-values.yaml
```

### Key Values

```yaml
# values.yaml
replicaCount: 3

config:
  proxy:
    upstreamAddr: "temporal-frontend:7233"
  sinks:
    otel:
      enabled: true
      endpoint: "otel-collector:4317"
    slack:
      enabled: true

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10

serviceMonitor:
  enabled: true
```

### Upgrade

```bash
helm upgrade temporal-profiler temporal-profiler/temporal-profiler \
  --namespace temporal-profiler \
  --reuse-values \
  --set image.tag=v1.1.0
```

## AWS ECS/Fargate

See [deploy/aws-ecs/README.md](../deploy/aws-ecs/README.md) for detailed instructions.

### Quick Start

```bash
cd deploy/aws-ecs

# Set environment variables
export AWS_ACCOUNT_ID=123456789012
export AWS_REGION=us-east-1

# Register task definition
envsubst < task-definition.json | aws ecs register-task-definition --cli-input-json file:///dev/stdin

# Create service
aws ecs create-service \
  --cluster temporal-cluster \
  --service-name temporal-profiler \
  --task-definition temporal-profiler \
  --desired-count 2 \
  --launch-type FARGATE
```

### SDK Connection (ECS)

```go
// From ECS tasks in the same VPC
client.Dial(client.Options{
    HostPort: "temporal-profiler.internal:7234",
})
```

## Google Cloud Run

See [deploy/gcp-cloudrun/README.md](../deploy/gcp-cloudrun/README.md) for detailed instructions.

### Quick Start

```bash
gcloud run deploy temporal-profiler \
  --image gcr.io/$PROJECT_ID/temporal-profiler \
  --platform managed \
  --region us-central1 \
  --port 7234 \
  --use-http2 \
  --min-instances 1 \
  --max-instances 10 \
  --vpc-connector projects/$PROJECT_ID/locations/$REGION/connectors/temporal-connector \
  --vpc-egress private-ranges-only
```

**Note:** Cloud Run requires `--use-http2` for gRPC support.

## Azure Container Apps

See [deploy/azure-container-apps/README.md](../deploy/azure-container-apps/README.md) for detailed instructions.

### Quick Start

```bash
az deployment group create \
  --resource-group temporal-profiler-rg \
  --template-file deploy/azure-container-apps/main.bicep \
  --parameters keyVaultName=temporal-profiler-kv
```

## Configuration Reference

### Environment Variables

All configuration can be set via environment variables with the `TEMPORAL_PROFILER_` prefix:

| Variable | Description | Default |
|----------|-------------|---------|
| `TEMPORAL_PROFILER_PROXY_LISTEN_ADDR` | Proxy listen address | `:7234` |
| `TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR` | Temporal server address | `localhost:7233` |
| `TEMPORAL_PROFILER_PROXY_TLS_ENABLED` | Enable TLS | `false` |
| `TEMPORAL_PROFILER_PROFILER_BUFFER_SIZE` | Ring buffer size | `10000` |
| `TEMPORAL_PROFILER_PROFILER_BATCH_SIZE` | Batch size for sinks | `100` |
| `TEMPORAL_PROFILER_PROFILER_FLUSH_INTERVAL` | Flush interval | `5s` |
| `TEMPORAL_PROFILER_SINKS_OTEL_ENABLED` | Enable OTEL export | `true` |
| `TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT` | OTEL collector address | `localhost:4317` |
| `TEMPORAL_PROFILER_SINKS_SLACK_ENABLED` | Enable Slack alerts | `false` |
| `TEMPORAL_PROFILER_SINKS_SLACK_WEBHOOK_URL` | Slack webhook URL | - |
| `TEMPORAL_PROFILER_LOGGING_LEVEL` | Log level | `info` |

### Config File

```yaml
# temporal-profiler.yaml
proxy:
  listen_addr: ":7234"
  upstream_addr: "temporal:7233"
  tls:
    enabled: false
    cert_file: "/certs/tls.crt"
    key_file: "/certs/tls.key"
    ca_file: "/certs/ca.crt"

profiler:
  buffer_size: 10000
  batch_size: 100
  flush_interval: "5s"
  worker_count: 2

sinks:
  otel:
    enabled: true
    endpoint: "otel-collector:4317"
    metrics:
      enabled: true
    traces:
      enabled: true

  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
    alerts:
      - name: "workflow_failed"
        condition: "event_type == WORKFLOW_FAILED"
        severity: "error"

admin:
  enabled: true
  addr: ":8080"
```

## SDK Integration

### Change One Line

The only change required in your SDK is to point to the profiler instead of the Temporal server:

**Go SDK:**
```go
// Before
client.Dial(client.Options{HostPort: "temporal:7233"})

// After
client.Dial(client.Options{HostPort: "temporal-profiler:7234"})
```

**TypeScript SDK:**
```typescript
// Before
await Connection.connect({ address: 'temporal:7233' });

// After
await Connection.connect({ address: 'temporal-profiler:7234' });
```

**Python SDK:**
```python
# Before
await Client.connect("temporal:7233")

# After
await Client.connect("temporal-profiler:7234")
```

**Java SDK:**
```java
// Before
WorkflowServiceStubs.newServiceStubs(
    WorkflowServiceStubsOptions.newBuilder()
        .setTarget("temporal:7233")
        .build());

// After
WorkflowServiceStubs.newServiceStubs(
    WorkflowServiceStubsOptions.newBuilder()
        .setTarget("temporal-profiler:7234")
        .build());
```

## TLS/mTLS Setup

### Generate Certificates (Development)

```bash
# Generate CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 365 -key ca.key -out ca.crt \
  -subj "/CN=Temporal Profiler CA"

# Generate server certificate
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=temporal-profiler"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt \
  -extfile <(echo "subjectAltName=DNS:temporal-profiler,DNS:localhost")
```

### Enable TLS

```yaml
proxy:
  tls:
    enabled: true
    cert_file: "/certs/server.crt"
    key_file: "/certs/server.key"
    ca_file: "/certs/ca.crt"  # Enables client verification (mTLS)
```

## Monitoring

### Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/health` | Liveness probe |
| `/ready` | Readiness probe |
| `/metrics` | Prometheus metrics |
| `/stats` | JSON statistics |

### Prometheus Metrics

Key metrics exported:

| Metric | Type | Description |
|--------|------|-------------|
| `temporal_profiler_events_processed_total` | Counter | Total events processed |
| `temporal_profiler_events_dropped_total` | Counter | Events dropped (buffer full) |
| `temporal_profiler_buffer_size` | Gauge | Current buffer size |
| `temporal_profiler_proxy_latency_seconds` | Histogram | Proxy latency |
| `temporal_profiler_sink_errors_total` | Counter | Sink errors by sink name |

### Grafana Dashboard

Import the dashboard from `deploy/docker/grafana/provisioning/dashboards/`.

## Troubleshooting

### Common Issues

**1. SDK Cannot Connect**
```bash
# Check profiler is running
curl http://profiler-host:8080/ready

# Check upstream connectivity
kubectl exec -it temporal-profiler-xxx -- wget -O - temporal:7233
```

**2. Events Not Appearing in OTEL**
```bash
# Check OTEL endpoint connectivity
kubectl exec -it temporal-profiler-xxx -- nc -zv otel-collector 4317

# Check profiler logs
kubectl logs -l app.kubernetes.io/name=temporal-profiler
```

**3. High Latency**
```bash
# Check buffer utilization
curl http://profiler-host:8080/stats | jq .buffer

# Increase buffer size or worker count
TEMPORAL_PROFILER_PROFILER_BUFFER_SIZE=50000
TEMPORAL_PROFILER_PROFILER_WORKER_COUNT=4
```

**4. Events Being Dropped**
```bash
# Check dropped count
curl http://profiler-host:8080/metrics | grep dropped

# Scale up or increase buffer size
kubectl scale deployment temporal-profiler --replicas=5
```

### Debug Mode

```bash
# Enable debug logging
TEMPORAL_PROFILER_LOGGING_LEVEL=debug
```

### Health Check Commands

```bash
# Liveness
curl -f http://localhost:8080/health

# Readiness
curl -f http://localhost:8080/ready

# Metrics
curl http://localhost:8080/metrics

# Stats (JSON)
curl http://localhost:8080/stats
```
