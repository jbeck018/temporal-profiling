# AWS ECS/Fargate Deployment

This directory contains configuration for deploying Temporal Profiler on AWS ECS with Fargate.

## Prerequisites

1. AWS CLI configured with appropriate credentials
2. ECR repository for the container image
3. VPC with private subnets
4. Temporal server accessible from the VPC
5. OTEL collector accessible from the VPC
6. AWS Secrets Manager secret for Slack webhook (optional)

## Setup

### 1. Create ECR Repository

```bash
aws ecr create-repository --repository-name temporal-profiler
```

### 2. Build and Push Image

```bash
# Authenticate Docker to ECR
aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com

# Build and push
docker build -t temporal-profiler ../../
docker tag temporal-profiler:latest $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/temporal-profiler:latest
docker push $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/temporal-profiler:latest
```

### 3. Create Secrets

```bash
# Create Slack webhook secret
aws secretsmanager create-secret \
  --name temporal-profiler/slack-webhook \
  --secret-string "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
```

### 4. Create IAM Roles

```bash
# Task execution role (for pulling images and secrets)
aws iam create-role \
  --role-name ecsTaskExecutionRole \
  --assume-role-policy-document file://trust-policy.json

# Task role (for application permissions)
aws iam create-role \
  --role-name temporal-profiler-task-role \
  --assume-role-policy-document file://trust-policy.json
```

### 5. Create Security Group

```bash
aws ec2 create-security-group \
  --group-name temporal-profiler-sg \
  --description "Security group for Temporal Profiler" \
  --vpc-id $VPC_ID

# Allow inbound gRPC from workers
aws ec2 authorize-security-group-ingress \
  --group-id $SG_ID \
  --protocol tcp \
  --port 7234 \
  --source-group $WORKER_SG_ID

# Allow outbound to Temporal server
aws ec2 authorize-security-group-egress \
  --group-id $SG_ID \
  --protocol tcp \
  --port 7233 \
  --cidr $TEMPORAL_CIDR
```

### 6. Deploy

```bash
# Replace variables in task definition
envsubst < task-definition.json > task-definition-final.json

# Register task definition
aws ecs register-task-definition --cli-input-json file://task-definition-final.json

# Create service
envsubst < service-definition.json > service-definition-final.json
aws ecs create-service --cli-input-json file://service-definition-final.json
```

## Auto Scaling

Configure auto scaling based on CPU utilization:

```bash
# Register scalable target
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --resource-id service/temporal-cluster/temporal-profiler \
  --scalable-dimension ecs:service:DesiredCount \
  --min-capacity 2 \
  --max-capacity 10

# Create scaling policy
aws application-autoscaling put-scaling-policy \
  --service-namespace ecs \
  --resource-id service/temporal-cluster/temporal-profiler \
  --scalable-dimension ecs:service:DesiredCount \
  --policy-name cpu-tracking \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration file://scaling-policy.json
```

## SDK Connection

From your application running in ECS, connect to the profiler:

```go
// Go SDK
client.Dial(client.Options{
    HostPort: "temporal-profiler.internal:7234",
})
```

```typescript
// TypeScript SDK
await Connection.connect({
    address: 'temporal-profiler.internal:7234',
});
```

```python
# Python SDK
await Client.connect("temporal-profiler.internal:7234")
```

## Monitoring

- **CloudWatch Logs**: `/ecs/temporal-profiler`
- **Container Insights**: Enable on ECS cluster for enhanced metrics
- **X-Ray**: Enable tracing for distributed traces (integrates with OTEL)

## Troubleshooting

```bash
# View service events
aws ecs describe-services --cluster temporal-cluster --services temporal-profiler

# View task logs
aws logs tail /ecs/temporal-profiler --follow

# Execute command in container
aws ecs execute-command \
  --cluster temporal-cluster \
  --task $TASK_ID \
  --container temporal-profiler \
  --interactive \
  --command "/bin/sh"
```
