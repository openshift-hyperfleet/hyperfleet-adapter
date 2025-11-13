# HyperFleet Adapter Helm Chart

This Helm chart deploys HyperFleet Adapter Framework services to Kubernetes.

## Features

- **Single Image, Multiple Adapters**: One container image, multiple adapter types (validation, dns, placement)
- **Config-Driven**: Each adapter runs with its own configuration
- **ServiceAccount Authentication**: Automatic in-cluster Kubernetes authentication
- **RBAC Support**: Per-adapter service accounts and permissions
- **Autoscaling**: Horizontal Pod Autoscaling per adapter type
- **Observability**: Prometheus metrics, health checks, optional tracing
- **Multi-Environment**: Dev, staging, and production configurations

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    HyperFleet Adapter                        │
│                    (Single Container Image)                  │
└────────────┬────────────────┬────────────────┬──────────────┘
             │                │                │
    ┌────────▼────────┐ ┌────▼─────────┐ ┌───▼──────────┐
    │   Validation    │ │     DNS      │ │  Placement   │
    │    Adapter      │ │   Adapter    │ │   Adapter    │
    │                 │ │              │ │              │
    │  • Config       │ │  • Config    │ │  • Config    │
    │  • ServiceAccount│ │  • ServiceAccount│ │  • ServiceAccount│
    │  • RBAC         │ │  • RBAC      │ │  • RBAC      │
    │  • Metrics      │ │  • Metrics   │ │  • Metrics   │
    └─────────────────┘ └──────────────┘ └──────────────┘
```

## Prerequisites

- Kubernetes 1.23+
- Helm 3.8+
- HyperFleet API deployed
- Message broker (RabbitMQ, Pub/Sub, or SQS) configured

## Installation

### Quick Start

```bash
# Create namespace
kubectl create namespace hyperfleet-system

# Create API token secret
kubectl create secret generic hyperfleet-api-token \
  --from-literal=token=<YOUR_API_TOKEN> \
  -n hyperfleet-system

# Create broker credentials (for RabbitMQ)
kubectl create secret generic rabbitmq-credentials \
  --from-literal=password=<RABBITMQ_PASSWORD> \
  -n hyperfleet-system

# Install chart (development)
helm install hyperfleet-adapter ./charts \
  -f charts/values-dev.yaml \
  --namespace hyperfleet-system
```

### Environment-Specific Installations

#### Development
```bash
helm install hyperfleet-adapter ./charts \
  -f charts/values-dev.yaml \
  --namespace hyperfleet-system \
  --create-namespace
```

#### Staging
```bash
helm install hyperfleet-adapter ./charts \
  -f charts/values-staging.yaml \
  --namespace hyperfleet-system
```

#### Production
```bash
helm install hyperfleet-adapter ./charts \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system
```

## Configuration

### Values Files

- `values.yaml` - Default values
- `values-dev.yaml` - Development overrides (debug logging, 1 replica)
- `values-staging.yaml` - Staging overrides (info logging, 2 replicas)
- `values-prod.yaml` - Production overrides (warn logging, 3+ replicas, autoscaling)

### Key Configuration Sections

#### Global Settings
```yaml
global:
  imageRegistry: quay.io/openshift-hyperfleet
  namespace: hyperfleet-system

environment: production
```

#### Image Configuration
```yaml
image:
  repository: hyperfleet-adapter
  tag: "1.0.0"
  pullPolicy: IfNotPresent
```

#### HyperFleet API
```yaml
hyperfleetApi:
  baseUrl: "http://hyperfleet-api.hyperfleet-system.svc.cluster.local:8080"
  version: "v1"
  tokenSecretName: "hyperfleet-api-token"
```

#### Broker Configuration
```yaml
broker:
  type: "rabbitmq"  # or "pubsub", "awsSqs"
  maxConcurrency: 100
  rabbitmq:
    host: "rabbitmq.hyperfleet-system.svc.cluster.local"
    port: "5672"
    queueName: "hyperfleet-cluster-events"
    username: "hyperfleet"
    passwordSecretName: "rabbitmq-credentials"
```

#### Adapter Definitions
```yaml
adapters:
  validation:
    enabled: true
    replicas: 2
    subscriptionName: "validation-adapter-sub"
    configFile: "validation-adapter-config.yaml"
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 10
```

### Adapter Configuration Files

Place adapter-specific YAML configurations in one of two ways:

#### Option 1: External Files (Recommended)
Create `configs/` directory with adapter configuration files:

```bash
charts/
  configs/
    validation-adapter-config.yaml
    dns-adapter-config.yaml
    placement-adapter-config.yaml
```

#### Option 2: Inline in Values
```yaml
adapterConfigs:
  validation: |
    adapterName: validation-adapter
    filters:
      eventTypes:
        - "cluster.created"
        - "cluster.updated"
    # ... rest of config
```

## RBAC

The chart automatically creates:
- ServiceAccount per adapter
- ClusterRole with required permissions
- ClusterRoleBinding connecting ServiceAccount to ClusterRole

### Permissions Granted

- **Namespaces**: create, get, list, watch, update, patch, delete
- **Core Resources**: pods, services, configmaps, secrets, serviceaccounts
- **Apps**: deployments, statefulsets, daemonsets, replicasets
- **Batch**: jobs, cronjobs
- **Networking**: ingresses, networkpolicies
- **RBAC**: roles, rolebindings (if adapter creates them)

### Switching to Namespaced RBAC

To use Role instead of ClusterRole:

```yaml
rbac:
  create: true
  clusterRole: false  # Uses Role instead of ClusterRole
```

## Autoscaling

Enable Horizontal Pod Autoscaling per adapter:

```yaml
adapters:
  validation:
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 20
      targetCPUUtilizationPercentage: 70
      targetMemoryUtilizationPercentage: 80
```

## Monitoring

### Prometheus Metrics

Metrics are exposed on port 8080 at `/metrics`:

```
adapter_events_processed_total
adapter_event_processing_duration_seconds
adapter_api_calls_total
adapter_k8s_operations_total
go_memstats_alloc_bytes
```

### ServiceMonitor

Automatically created when `serviceMonitor.enabled: true`:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  scrapeTimeout: 10s
```

### Health Checks

- **Liveness**: `GET /healthz` on port 8081
- **Readiness**: `GET /readyz` on port 8081

## Upgrading

```bash
# Upgrade with new values
helm upgrade hyperfleet-adapter ./charts \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system

# Upgrade adapter config only
kubectl create configmap hyperfleet-adapter-validation-config \
  --from-file=adapter-config.yaml=./configs/validation-adapter-config.yaml \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl rollout restart deployment/hyperfleet-adapter-validation -n hyperfleet-system
```

## Rollback

```bash
# List releases
helm history hyperfleet-adapter --namespace hyperfleet-system

# Rollback to previous
helm rollback hyperfleet-adapter --namespace hyperfleet-system

# Rollback to specific revision
helm rollback hyperfleet-adapter 3 --namespace hyperfleet-system
```

## Uninstalling

```bash
helm uninstall hyperfleet-adapter --namespace hyperfleet-system
```

## Troubleshooting

### Check Pod Status
```bash
kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter
```

### View Logs
```bash
# Validation adapter logs
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation --tail=100 -f

# DNS adapter logs
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=dns --tail=100 -f
```

### Check Configuration
```bash
# View environment ConfigMap
kubectl get cm hyperfleet-adapter-environment -n hyperfleet-system -o yaml

# View adapter config
kubectl get cm hyperfleet-adapter-validation-config -n hyperfleet-system -o yaml
```

### Verify RBAC
```bash
# Check ServiceAccount
kubectl get sa -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Check ClusterRole
kubectl get clusterrole -l app.kubernetes.io/name=hyperfleet-adapter

# Test permissions
kubectl auth can-i create namespaces \
  --as=system:serviceaccount:hyperfleet-system:hyperfleet-adapter-validation
```

### Check Metrics
```bash
# Port-forward to metrics endpoint
kubectl port-forward -n hyperfleet-system \
  service/hyperfleet-adapter-validation 8080:8080

# View metrics
curl http://localhost:8080/metrics
```

## Advanced Configuration

### Custom Resource Limits
```yaml
adapters:
  validation:
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 2000m
        memory: 2Gi
```

### Node Affinity
```yaml
adapters:
  validation:
    affinity:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: workload-type
              operator: In
              values:
              - adapters
```

### Additional Environment Variables
```yaml
adapters:
  validation:
    env:
    - name: CUSTOM_VAR
      value: "custom-value"
    - name: SECRET_VAR
      valueFrom:
        secretKeyRef:
          name: my-secret
          key: secret-key
```

## Examples

See the `examples/` directory for:
- Complete deployment examples
- Sample adapter configurations
- Integration test scripts

## Support

For issues and questions:
- GitHub: https://github.com/openshift-hyperfleet/hyperfleet-adapter
- Documentation: See `/docs` directory

## License

Apache 2.0 - See LICENSE file
