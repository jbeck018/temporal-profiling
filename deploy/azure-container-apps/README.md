# Azure Container Apps Deployment

This directory contains configuration for deploying Temporal Profiler on Azure Container Apps.

## Prerequisites

1. Azure CLI installed and configured
2. Azure subscription with Container Apps enabled
3. Azure Key Vault for secrets management
4. VNet with subnet for Container Apps (optional, for private networking)
5. Temporal server accessible from the Container Apps environment

## Setup

### 1. Create Resource Group

```bash
az group create --name temporal-profiler-rg --location eastus
```

### 2. Create Key Vault and Add Secrets

```bash
# Create Key Vault
az keyvault create \
  --name temporal-profiler-kv \
  --resource-group temporal-profiler-rg \
  --location eastus

# Add Slack webhook secret
az keyvault secret set \
  --vault-name temporal-profiler-kv \
  --name slack-webhook-url \
  --value "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
```

### 3. Create Container Apps Environment (Optional: With VNet)

```bash
# Create VNet and subnet
az network vnet create \
  --name temporal-vnet \
  --resource-group temporal-profiler-rg \
  --location eastus \
  --address-prefix 10.0.0.0/16

az network vnet subnet create \
  --name container-apps-subnet \
  --vnet-name temporal-vnet \
  --resource-group temporal-profiler-rg \
  --address-prefix 10.0.0.0/23

# Get subnet ID
SUBNET_ID=$(az network vnet subnet show \
  --name container-apps-subnet \
  --vnet-name temporal-vnet \
  --resource-group temporal-profiler-rg \
  --query id -o tsv)
```

### 4. Deploy with Bicep

```bash
# Deploy without VNet
az deployment group create \
  --resource-group temporal-profiler-rg \
  --template-file main.bicep \
  --parameters \
    keyVaultName=temporal-profiler-kv \
    temporalServerAddr="temporal.internal:7233" \
    otelEndpoint="otel-collector.internal:4317"

# Deploy with VNet
az deployment group create \
  --resource-group temporal-profiler-rg \
  --template-file main.bicep \
  --parameters \
    keyVaultName=temporal-profiler-kv \
    temporalServerAddr="temporal.internal:7233" \
    otelEndpoint="otel-collector.internal:4317" \
    subnetId=$SUBNET_ID
```

### 5. Alternative: Deploy with Azure CLI

```bash
# Create Container Apps environment
az containerapp env create \
  --name temporal-profiler-env \
  --resource-group temporal-profiler-rg \
  --location eastus

# Create Container App
az containerapp create \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg \
  --environment temporal-profiler-env \
  --image ghcr.io/temporal-profiling/temporal-profiler:latest \
  --target-port 7234 \
  --transport http2 \
  --ingress internal \
  --min-replicas 2 \
  --max-replicas 10 \
  --cpu 1.0 \
  --memory 2.0Gi \
  --env-vars \
    TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=temporal.internal:7233 \
    TEMPORAL_PROFILER_SINKS_OTEL_ENABLED=true \
    TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT=otel-collector.internal:4317
```

## SDK Connection

From services within the same Container Apps environment or VNet:

```go
// Go SDK
client.Dial(client.Options{
    HostPort: "temporal-profiler.internal.azurecontainerapps.io:7234",
})
```

```typescript
// TypeScript SDK
await Connection.connect({
    address: 'temporal-profiler.internal.azurecontainerapps.io:7234',
});
```

```python
# Python SDK
await Client.connect("temporal-profiler.internal.azurecontainerapps.io:7234")
```

## Monitoring

- **Log Analytics**: Logs are sent to the configured Log Analytics workspace
- **Azure Monitor**: Built-in metrics for container apps
- **Application Insights**: Enable for advanced monitoring

```bash
# View logs
az containerapp logs show \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg \
  --follow

# View revisions
az containerapp revision list \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg
```

## Scaling

The deployment includes CPU and memory-based autoscaling. To customize:

```bash
az containerapp update \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg \
  --min-replicas 3 \
  --max-replicas 20 \
  --scale-rule-name cpu-rule \
  --scale-rule-type cpu \
  --scale-rule-metadata type=Utilization value=60
```

## Troubleshooting

```bash
# Describe app
az containerapp show \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg

# View system logs
az containerapp logs show \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg \
  --type system

# Restart app
az containerapp revision restart \
  --name temporal-profiler \
  --resource-group temporal-profiler-rg \
  --revision <revision-name>
```
