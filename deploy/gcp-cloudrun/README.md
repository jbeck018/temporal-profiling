# Google Cloud Run Deployment

This directory contains configuration for deploying Temporal Profiler on Google Cloud Run.

## Prerequisites

1. Google Cloud SDK (`gcloud`) installed and configured
2. Artifact Registry repository for container images
3. VPC connector for private network access
4. Temporal server accessible via VPC
5. Cloud Run API enabled

## Important Notes

- Cloud Run requires HTTP/2 for gRPC (`h2c` port name)
- VPC connector needed for private network access to Temporal server
- Admin port (8080) requires separate ingress configuration
- Cloud Run has a maximum request timeout of 60 minutes

## Setup

### 1. Enable APIs

```bash
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  secretmanager.googleapis.com \
  vpcaccess.googleapis.com
```

### 2. Create Artifact Registry Repository

```bash
gcloud artifacts repositories create temporal-profiler \
  --repository-format=docker \
  --location=$REGION \
  --description="Temporal Profiler container images"
```

### 3. Build and Push Image

```bash
# Configure Docker for Artifact Registry
gcloud auth configure-docker $REGION-docker.pkg.dev

# Build and push
docker build -t $REGION-docker.pkg.dev/$PROJECT_ID/temporal-profiler/temporal-profiler:latest ../../
docker push $REGION-docker.pkg.dev/$PROJECT_ID/temporal-profiler/temporal-profiler:latest

# Or use Cloud Build
gcloud builds submit --tag $REGION-docker.pkg.dev/$PROJECT_ID/temporal-profiler/temporal-profiler:latest ../../
```

### 4. Create VPC Connector

```bash
gcloud compute networks vpc-access connectors create temporal-connector \
  --region=$REGION \
  --network=$VPC_NETWORK \
  --range=10.8.0.0/28 \
  --min-instances=2 \
  --max-instances=10
```

### 5. Create Secrets

```bash
# Create secret for Slack webhook
echo -n "https://hooks.slack.com/services/YOUR/WEBHOOK/URL" | \
  gcloud secrets create slack-webhook-url --data-file=-

# Grant access to Cloud Run service account
gcloud secrets add-iam-policy-binding slack-webhook-url \
  --member="serviceAccount:temporal-profiler@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

### 6. Create Service Account

```bash
gcloud iam service-accounts create temporal-profiler \
  --display-name="Temporal Profiler Service Account"

# Grant necessary permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:temporal-profiler@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/logging.logWriter"
```

### 7. Deploy Service

```bash
# Replace variables
export PROJECT_ID=$(gcloud config get-value project)
export REGION=us-central1
export VPC_CONNECTOR=temporal-connector

envsubst < service.yaml > service-final.yaml

# Deploy
gcloud run services replace service-final.yaml --region=$REGION

# Or deploy directly with gcloud
gcloud run deploy temporal-profiler \
  --image $REGION-docker.pkg.dev/$PROJECT_ID/temporal-profiler/temporal-profiler:latest \
  --platform managed \
  --region $REGION \
  --port 7234 \
  --use-http2 \
  --min-instances 1 \
  --max-instances 10 \
  --cpu 2 \
  --memory 1Gi \
  --set-env-vars "TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR=temporal:7233" \
  --vpc-connector projects/$PROJECT_ID/locations/$REGION/connectors/temporal-connector \
  --vpc-egress private-ranges-only \
  --service-account temporal-profiler@$PROJECT_ID.iam.gserviceaccount.com \
  --no-allow-unauthenticated
```

## SDK Connection

From services within the same VPC:

```go
// Go SDK
client.Dial(client.Options{
    HostPort: "temporal-profiler-xxxx-uc.a.run.app:443",
    ConnectionOptions: client.ConnectionOptions{
        TLS: &tls.Config{},
    },
})
```

For internal traffic, use Cloud Run's internal URL:

```go
// Go SDK with internal URL
client.Dial(client.Options{
    HostPort: "temporal-profiler.internal:7234",
})
```

## Monitoring

- **Cloud Logging**: Logs are automatically sent to Cloud Logging
- **Cloud Monitoring**: Built-in metrics for request count, latency, and errors
- **Cloud Trace**: Enable for distributed tracing

```bash
# View logs
gcloud logging read "resource.type=cloud_run_revision AND resource.labels.service_name=temporal-profiler" --limit=100

# View metrics
gcloud monitoring dashboards create --config-from-file=dashboard.json
```

## Troubleshooting

```bash
# Describe service
gcloud run services describe temporal-profiler --region=$REGION

# List revisions
gcloud run revisions list --service=temporal-profiler --region=$REGION

# View logs
gcloud run services logs read temporal-profiler --region=$REGION --limit=100
```
