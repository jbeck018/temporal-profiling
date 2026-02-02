// Azure Container Apps deployment for Temporal Profiler
// Deploy with: az deployment group create --resource-group <rg> --template-file main.bicep

@description('Location for all resources')
param location string = resourceGroup().location

@description('Name of the Container Apps environment')
param environmentName string = 'temporal-profiler-env'

@description('Name of the Container App')
param appName string = 'temporal-profiler'

@description('Container image')
param containerImage string = 'ghcr.io/temporal-profiling/temporal-profiler:latest'

@description('Temporal server address')
param temporalServerAddr string = 'temporal.internal:7233'

@description('OTEL collector endpoint')
param otelEndpoint string = 'otel-collector.internal:4317'

@description('Minimum replicas')
param minReplicas int = 2

@description('Maximum replicas')
param maxReplicas int = 10

@description('Name of the Key Vault containing secrets')
param keyVaultName string

@description('VNet subnet resource ID for Container Apps environment')
param subnetId string = ''

// Log Analytics workspace for Container Apps
resource logAnalytics 'Microsoft.OperationalInsights/workspaces@2022-10-01' = {
  name: '${environmentName}-logs'
  location: location
  properties: {
    sku: {
      name: 'PerGB2018'
    }
    retentionInDays: 30
  }
}

// Container Apps Environment
resource containerAppEnvironment 'Microsoft.App/managedEnvironments@2023-05-01' = {
  name: environmentName
  location: location
  properties: {
    appLogsConfiguration: {
      destination: 'log-analytics'
      logAnalyticsConfiguration: {
        customerId: logAnalytics.properties.customerId
        sharedKey: logAnalytics.listKeys().primarySharedKey
      }
    }
    vnetConfiguration: subnetId != '' ? {
      infrastructureSubnetId: subnetId
      internal: true
    } : null
  }
}

// Reference to existing Key Vault
resource keyVault 'Microsoft.KeyVault/vaults@2023-02-01' existing = {
  name: keyVaultName
}

// Temporal Profiler Container App
resource temporalProfiler 'Microsoft.App/containerApps@2023-05-01' = {
  name: appName
  location: location
  identity: {
    type: 'SystemAssigned'
  }
  properties: {
    managedEnvironmentId: containerAppEnvironment.id
    configuration: {
      activeRevisionsMode: 'Single'
      ingress: {
        external: false
        targetPort: 7234
        transport: 'http2'
        traffic: [
          {
            weight: 100
            latestRevision: true
          }
        ]
      }
      secrets: [
        {
          name: 'slack-webhook-url'
          keyVaultUrl: '${keyVault.properties.vaultUri}secrets/slack-webhook-url'
          identity: 'system'
        }
      ]
      registries: []
    }
    template: {
      containers: [
        {
          name: 'temporal-profiler'
          image: containerImage
          resources: {
            cpu: json('1.0')
            memory: '2Gi'
          }
          env: [
            {
              name: 'TEMPORAL_PROFILER_PROXY_LISTEN_ADDR'
              value: ':7234'
            }
            {
              name: 'TEMPORAL_PROFILER_PROXY_UPSTREAM_ADDR'
              value: temporalServerAddr
            }
            {
              name: 'TEMPORAL_PROFILER_SINKS_OTEL_ENABLED'
              value: 'true'
            }
            {
              name: 'TEMPORAL_PROFILER_SINKS_OTEL_ENDPOINT'
              value: otelEndpoint
            }
            {
              name: 'TEMPORAL_PROFILER_ADMIN_ENABLED'
              value: 'true'
            }
            {
              name: 'TEMPORAL_PROFILER_ADMIN_ADDR'
              value: ':8080'
            }
            {
              name: 'TEMPORAL_PROFILER_SINKS_SLACK_ENABLED'
              value: 'true'
            }
            {
              name: 'TEMPORAL_PROFILER_SINKS_SLACK_WEBHOOK_URL'
              secretRef: 'slack-webhook-url'
            }
          ]
          probes: [
            {
              type: 'Liveness'
              httpGet: {
                path: '/health'
                port: 8080
              }
              periodSeconds: 10
              failureThreshold: 3
              initialDelaySeconds: 5
            }
            {
              type: 'Readiness'
              httpGet: {
                path: '/ready'
                port: 8080
              }
              periodSeconds: 5
              failureThreshold: 3
              initialDelaySeconds: 5
            }
            {
              type: 'Startup'
              httpGet: {
                path: '/health'
                port: 8080
              }
              periodSeconds: 5
              failureThreshold: 30
              initialDelaySeconds: 5
            }
          ]
        }
      ]
      scale: {
        minReplicas: minReplicas
        maxReplicas: maxReplicas
        rules: [
          {
            name: 'cpu-scaling'
            custom: {
              type: 'cpu'
              metadata: {
                type: 'Utilization'
                value: '70'
              }
            }
          }
          {
            name: 'memory-scaling'
            custom: {
              type: 'memory'
              metadata: {
                type: 'Utilization'
                value: '80'
              }
            }
          }
        ]
      }
    }
  }
}

// Grant Key Vault access to Container App
resource keyVaultAccessPolicy 'Microsoft.KeyVault/vaults/accessPolicies@2023-02-01' = {
  parent: keyVault
  name: 'add'
  properties: {
    accessPolicies: [
      {
        tenantId: subscription().tenantId
        objectId: temporalProfiler.identity.principalId
        permissions: {
          secrets: [
            'get'
          ]
        }
      }
    ]
  }
}

// Outputs
output appFqdn string = temporalProfiler.properties.configuration.ingress.fqdn
output appUrl string = 'https://${temporalProfiler.properties.configuration.ingress.fqdn}'
output environmentId string = containerAppEnvironment.id
