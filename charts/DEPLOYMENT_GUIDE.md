# HyperFleet Adapter - Deployment Guide

Complete guide for deploying and managing HyperFleet adapters on Kubernetes.

---

## 📋 Quick Reference

### Installation

```bash
# Development
helm install hyperfleet-adapter ./charts -f charts/values-dev.yaml -n hyperfleet-system --create-namespace

# Staging
helm install hyperfleet-adapter ./charts -f charts/values-staging.yaml -n hyperfleet-system

# Production
helm install hyperfleet-adapter ./charts -f charts/values-prod.yaml -n hyperfleet-system
```

### Common Commands

<details>
<summary>Click to expand: Common kubectl and helm commands</summary>

```bash
# List pods
kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# View logs
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -f --tail=100

# Check health
POD=$(kubectl get pod -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n hyperfleet-system $POD -- curl -s http://localhost:8081/healthz

# Upgrade
helm upgrade hyperfleet-adapter ./charts -f charts/values-prod.yaml -n hyperfleet-system

# Rollback
helm rollback hyperfleet-adapter -n hyperfleet-system
```

</details>

### Port Reference

- **8080**: Metrics (Prometheus)
- **8081**: Health checks (`/healthz`, `/readyz`)

### Useful Labels

```yaml
app.kubernetes.io/name: hyperfleet-adapter          # All adapters
hyperfleet.io/adapter-type: validation              # Specific adapter
app.kubernetes.io/instance: hyperfleet-adapter      # Helm instance
```

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Environment Setup](#environment-setup)
3. [Deploy to Development](#deploy-to-development)
4. [Deploy to Staging](#deploy-to-staging)
5. [Deploy to Production](#deploy-to-production)
6. [Verification](#verification)
7. [Monitoring](#monitoring)
8. [Operations](#operations)
9. [Troubleshooting](#troubleshooting)
10. [Best Practices](#best-practices)

---

## Prerequisites

### Required Components

- ✅ Kubernetes cluster 1.23+ with `kubectl` access
- ✅ Helm 3.8+ installed
- ✅ HyperFleet API deployed and accessible
- ✅ Message broker (RabbitMQ, Google Pub/Sub, or AWS SQS) configured
- ✅ Prometheus Operator (optional, for metrics)

### Verify Prerequisites

```bash
# Check kubectl access
kubectl version --client
kubectl cluster-info

# Check Helm
helm version

# Check namespace access
kubectl get namespace hyperfleet-system || kubectl create namespace hyperfleet-system
```

---

## Environment Setup

### 1. Create Secrets

#### HyperFleet API Token

```bash
# Create API token secret
kubectl create secret generic hyperfleet-api-token \
  --from-literal=token=YOUR_API_TOKEN_HERE \
  --namespace hyperfleet-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Message Broker Credentials

**For RabbitMQ:**
```bash
kubectl create secret generic rabbitmq-credentials \
  --from-literal=password=YOUR_RABBITMQ_PASSWORD \
  --namespace hyperfleet-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

**For Google Cloud Pub/Sub:**
```bash
kubectl create secret generic pubsub-credentials \
  --from-file=key.json=./path/to/service-account-key.json \
  --namespace hyperfleet-system
```

### 2. Prepare Adapter Configurations

Create adapter configuration files in `charts/configs/` directory:

```bash
cd /path/to/hyperfleet-adapter
mkdir -p charts/configs
```

Example structure:
```
charts/
  configs/
    validation-adapter-config.yaml
    dns-adapter-config.yaml
    placement-adapter-config.yaml
```

See the [adapter-config-template.yaml](../data/adapter-config-template.yaml) for configuration structure.

### 3. Customize Values

Copy the appropriate values file and customize:

```bash
# For development
cp charts/values-dev.yaml charts/values-dev-custom.yaml
vim charts/values-dev-custom.yaml

# Update:
# - global.imageRegistry (if using private registry)
# - image.tag (specific version)
# - hyperfleetApi.baseUrl (your API URL)
# - broker configuration (match your broker setup)
# - adapters.*.enabled (enable/disable adapters)
```

---

## Deploy to Development

### Step 1: Validate Configuration

```bash
# Lint chart
helm lint ./charts -f charts/values-dev.yaml

# Dry-run to check for errors
helm install hyperfleet-adapter ./charts \
  -f charts/values-dev.yaml \
  --namespace hyperfleet-system \
  --dry-run --debug

# Template rendering (preview manifests)
helm template hyperfleet-adapter ./charts \
  -f charts/values-dev.yaml \
  --namespace hyperfleet-system
```

### Step 2: Install Chart

```bash
# Install with development values
helm install hyperfleet-adapter ./charts \
  -f charts/values-dev.yaml \
  --namespace hyperfleet-system \
  --create-namespace
```

### Step 3: Watch Deployment

```bash
# Watch pods coming up
kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter -w

# Check deployment status
kubectl rollout status deployment/hyperfleet-adapter-validation -n hyperfleet-system
kubectl rollout status deployment/hyperfleet-adapter-dns -n hyperfleet-system
```

### Step 4: Check Logs

```bash
# Validation adapter logs
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation --tail=50

# DNS adapter logs
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=dns --tail=50

# Follow logs in real-time
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -f
```

---

## Deploy to Staging

### Step 1: Update Image Tag

```bash
# Build and push image
docker build -t quay.io/openshift-hyperfleet/hyperfleet-adapter:1.0.0 .
docker push quay.io/openshift-hyperfleet/hyperfleet-adapter:1.0.0
```

### Step 2: Install to Staging

```bash
# Install with staging values
helm install hyperfleet-adapter ./charts \
  -f charts/values-staging.yaml \
  --namespace hyperfleet-system
```

### Step 3: Verify Deployment

```bash
# Check all pods are running
kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Check services
kubectl get svc -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Check ServiceMonitors (if Prometheus Operator installed)
kubectl get servicemonitor -n hyperfleet-system
```

---

## Deploy to Production

### Pre-Deployment Checklist

- [ ] Image tagged with stable version (not `latest`)
- [ ] All secrets created and verified
- [ ] Adapter configurations reviewed and tested
- [ ] Resource limits appropriate for workload
- [ ] Autoscaling enabled and configured
- [ ] Monitoring and alerting configured
- [ ] Rollback plan documented

### Step 1: Review Production Values

```bash
# Review production configuration
cat charts/values-prod.yaml

# Ensure:
# - image.tag is stable version (not "latest")
# - observability.logLevel is "warn" or "error"
# - adapters.*.replicas >= 3
# - adapters.*.autoscaling.enabled is true
# - Resources are appropriate
```

### Step 2: Deploy to Production

```bash
# Install with production values
helm install hyperfleet-adapter ./charts \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system \
  --timeout 10m \
  --wait
```

### Step 3: Progressive Verification

<details>
<summary>Click to expand: Production verification steps</summary>

```bash
# 1. Check pod status
kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# 2. Check logs for errors (each adapter)
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation --tail=100 | grep -i error
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=dns --tail=100 | grep -i error

# 3. Check health endpoints
kubectl port-forward -n hyperfleet-system svc/hyperfleet-adapter-validation 8081:8081 &
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
pkill -f "port-forward.*8081"

# 4. Check metrics
kubectl port-forward -n hyperfleet-system svc/hyperfleet-adapter-validation 8080:8080 &
curl http://localhost:8080/metrics | grep adapter_events_processed_total
pkill -f "port-forward.*8080"

# 5. Monitor event processing
kubectl logs -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -f | grep "event processed"
```

</details>

### Step 4: Verify Autoscaling

```bash
# Check HPA status
kubectl get hpa -n hyperfleet-system

# Watch HPA scaling
kubectl get hpa -n hyperfleet-system -w

# Check HPA details
kubectl describe hpa hyperfleet-adapter-validation -n hyperfleet-system
```

---

## Verification

### 1. Component Health

```bash
# Check all resources
kubectl get all -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Expected output:
# - Deployments: validation, dns, (placement)
# - Pods: Multiple per deployment
# - Services: One per adapter
# - HPA: One per adapter (if enabled)
```

### 2. ServiceAccount and RBAC

```bash
# Check ServiceAccounts
kubectl get sa -n hyperfleet-system | grep hyperfleet-adapter

# Check ClusterRoles
kubectl get clusterrole | grep hyperfleet-adapter

# Check ClusterRoleBindings
kubectl get clusterrolebinding | grep hyperfleet-adapter

# Test permissions
kubectl auth can-i create jobs \
  --as=system:serviceaccount:hyperfleet-system:hyperfleet-adapter-validation
# Should return: yes

# Test namespace creation permission
kubectl auth can-i create namespaces \
  --as=system:serviceaccount:hyperfleet-system:hyperfleet-adapter-validation
# Should return: yes
```

### 3. Configuration

```bash
# Check ConfigMaps
kubectl get cm -n hyperfleet-system | grep hyperfleet-adapter

# View environment config
kubectl get cm hyperfleet-adapter-environment -n hyperfleet-system -o yaml

# View adapter config
kubectl get cm hyperfleet-adapter-validation-config -n hyperfleet-system -o jsonpath='{.data.adapter-config\.yaml}'

# Check configuration syntax
kubectl get cm hyperfleet-adapter-validation-config -n hyperfleet-system \
  -o jsonpath='{.data.adapter-config\.yaml}' | yq eval
```

### 4. Connectivity Tests

```bash
# Test API connectivity from adapter pod
POD=$(kubectl get pod -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -o jsonpath='{.items[0].metadata.name}')

# Test API endpoint
kubectl exec -n hyperfleet-system $POD -- curl -s http://hyperfleet-api.hyperfleet-system.svc.cluster.local:8080/health

# Test Kubernetes API access
kubectl exec -n hyperfleet-system $POD -- ls /var/run/secrets/kubernetes.io/serviceaccount/
# Should show: ca.crt, namespace, token
```

---

## Monitoring

### Prometheus Metrics

**Access metrics directly:**
```bash
# Port-forward to adapter
kubectl port-forward -n hyperfleet-system svc/hyperfleet-adapter-validation 8080:8080
curl http://localhost:8080/metrics
```

**Key metrics to monitor:**
- `adapter_events_processed_total` - Total events processed
- `adapter_event_processing_duration_seconds` - Processing time
- `adapter_api_calls_total` - API call count
- `adapter_k8s_operations_total` - Kubernetes operations

**Query in Prometheus:**
```bash
# Port-forward to Prometheus (if deployed)
kubectl port-forward -n monitoring svc/prometheus 9090:9090

# Open http://localhost:9090 and query metrics
```

### Grafana Dashboard

Import HyperFleet adapter dashboard:

```bash
# Port-forward to Grafana
kubectl port-forward -n monitoring svc/grafana 3000:3000

# Open http://localhost:3000
# Import dashboard from charts/grafana-dashboard.json
```

### ServiceMonitor

```bash
# Check ServiceMonitor
kubectl get servicemonitor -n hyperfleet-system

# Verify Prometheus targets
# Open Prometheus UI and check hyperfleet-adapter targets
```

### Logs Aggregation

If using ELK/Loki:

```bash
# Loki query
{namespace="hyperfleet-system", adapter_type="validation"}

# Elasticsearch query
namespace:"hyperfleet-system" AND adapter_type:"validation"
```

---

## Operations

### Upgrade Deployment

```bash
# Upgrade with new values
helm upgrade hyperfleet-adapter ./charts \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system

# Upgrade with specific parameters
helm upgrade hyperfleet-adapter ./charts \
  --set image.tag=1.1.0 \
  --set adapters.validation.replicas=5 \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system
```

### Update Configuration Only

```bash
# 1. Edit config file
vim charts/configs/validation-adapter-config.yaml

# 2. Update ConfigMap
kubectl create configmap hyperfleet-adapter-validation-config \
  --from-file=adapter-config.yaml=charts/configs/validation-adapter-config.yaml \
  --namespace hyperfleet-system \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Restart pods
kubectl rollout restart deployment/hyperfleet-adapter-validation -n hyperfleet-system

# 4. Verify
kubectl rollout status deployment/hyperfleet-adapter-validation -n hyperfleet-system
```

### Update Environment Variables

```bash
# Update environment config via Helm
helm upgrade hyperfleet-adapter ./charts \
  --set observability.logLevel=debug \
  --set broker.maxConcurrency=200 \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system
```

### Enable/Disable Adapter

```bash
# Enable placement adapter
helm upgrade hyperfleet-adapter ./charts \
  --set adapters.placement.enabled=true \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system

# Disable DNS adapter
helm upgrade hyperfleet-adapter ./charts \
  --set adapters.dns.enabled=false \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system
```

### Scaling

**Manual scaling:**
```bash
# Scale deployment
kubectl scale deployment/hyperfleet-adapter-validation --replicas=5 -n hyperfleet-system

# Via Helm
helm upgrade hyperfleet-adapter ./charts \
  --set adapters.validation.replicas=5 \
  -f charts/values-prod.yaml \
  --namespace hyperfleet-system
```

**Autoscaling:**
```bash
# Check HPA
kubectl get hpa -n hyperfleet-system

# Describe HPA for details
kubectl describe hpa hyperfleet-adapter-validation -n hyperfleet-system
```

### Rollback

```bash
# List release history
helm history hyperfleet-adapter --namespace hyperfleet-system

# Rollback to previous version
helm rollback hyperfleet-adapter --namespace hyperfleet-system

# Rollback to specific revision
helm rollback hyperfleet-adapter 3 --namespace hyperfleet-system
```

### Secrets Management

```bash
# Update API token
kubectl create secret generic hyperfleet-api-token \
  --from-literal=token=NEW_TOKEN \
  --namespace hyperfleet-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Update RabbitMQ credentials
kubectl create secret generic rabbitmq-credentials \
  --from-literal=password=NEW_PASSWORD \
  --namespace hyperfleet-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to use new secrets
kubectl rollout restart deployment -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter
```

---

## Troubleshooting

### Pods Not Starting

**Check pod events:**
```bash
# Describe pod
kubectl describe pod -n hyperfleet-system <pod-name>

# Check events
kubectl get events -n hyperfleet-system --sort-by='.lastTimestamp' | grep adapter
```

**Common issues:**
- **ImagePullBackOff** → Check image tag and registry access
- **CrashLoopBackOff** → Check logs for startup errors
  ```bash
  kubectl logs -n hyperfleet-system <pod-name>
  kubectl logs -n hyperfleet-system <pod-name> --previous  # Previous crashed pod
  ```
- **Pending** → Check resource availability
  ```bash
  kubectl describe node
  kubectl top nodes
  ```

### Configuration Errors

**Check logs for config errors:**
```bash
kubectl logs -n hyperfleet-system <pod-name> | grep -i "config\|error"
```

**Validate ConfigMap:**
```bash
# Extract and validate YAML syntax
kubectl get cm hyperfleet-adapter-validation-config -n hyperfleet-system \
  -o jsonpath='{.data.adapter-config\.yaml}' | yq eval
```

**Check configuration:**
```bash
# View environment config
kubectl get cm hyperfleet-adapter-environment -n hyperfleet-system -o yaml

# View broker config
kubectl get cm hyperfleet-adapter-environment -n hyperfleet-system -o yaml | grep BROKER
```

### Broker Connection Issues

**Check broker configuration:**
```bash
# View broker environment variables
kubectl get cm hyperfleet-adapter-environment -n hyperfleet-system -o yaml | grep BROKER

# Test broker connectivity
POD=$(kubectl get pod -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -o jsonpath='{.items[0].metadata.name}')

# For RabbitMQ
kubectl exec -n hyperfleet-system $POD -- nc -zv rabbitmq.hyperfleet-system.svc.cluster.local 5672

# Check logs for broker errors
kubectl logs -n hyperfleet-system $POD | grep -i "broker\|rabbitmq\|connection"
```

### RBAC Permissions Issues

**Check logs for permission errors:**
```bash
kubectl logs -n hyperfleet-system <pod-name> | grep -i "forbidden\|unauthorized"
```

**Verify permissions:**
```bash
# Test specific permission
kubectl auth can-i create namespaces \
  --as=system:serviceaccount:hyperfleet-system:hyperfleet-adapter-validation

# View ClusterRole rules
kubectl get clusterrole hyperfleet-adapter-validation -o yaml

# View ClusterRoleBinding
kubectl get clusterrolebinding hyperfleet-adapter-validation -o yaml
```

### Performance Issues

**Check resource usage:**
```bash
# Pod resource usage
kubectl top pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Node resource usage
kubectl top nodes
```

**Check HPA status:**
```bash
kubectl get hpa -n hyperfleet-system -o yaml
kubectl describe hpa hyperfleet-adapter-validation -n hyperfleet-system
```

**Increase resources:**
```bash
helm upgrade hyperfleet-adapter ./charts \
  -f charts/values-prod.yaml \
  --set adapters.validation.resources.limits.memory=2Gi \
  --set adapters.validation.resources.limits.cpu=2000m \
  --namespace hyperfleet-system
```

### Connectivity Issues

**Test API connectivity:**
```bash
POD=$(kubectl get pod -n hyperfleet-system -l hyperfleet.io/adapter-type=validation -o jsonpath='{.items[0].metadata.name}')

# Test API endpoint
kubectl exec -n hyperfleet-system $POD -- curl -v http://hyperfleet-api.hyperfleet-system.svc.cluster.local:8080/health

# Test external API (if applicable)
kubectl exec -n hyperfleet-system $POD -- curl -v https://api.hyperfleet.io/health
```

**Test Kubernetes API access:**
```bash
# Check ServiceAccount token
kubectl exec -n hyperfleet-system $POD -- ls -la /var/run/secrets/kubernetes.io/serviceaccount/

# Test K8s API call
kubectl exec -n hyperfleet-system $POD -- curl -k https://kubernetes.default.svc/api/v1/namespaces
```

---

## Best Practices

### Deployment

1. **Always test in dev/staging first** before production
2. **Use specific image tags** in production (not `latest`)
3. **Enable autoscaling** in production for high availability
4. **Set appropriate resource limits** based on workload
5. **Use Helm values files** for environment-specific configurations

### Configuration

6. **Validate configurations** before deployment (dry-run, lint)
7. **Document configuration changes** in version control
8. **Keep secrets encrypted** and never commit to Git
9. **Regular backup** of configurations and secrets
10. **Use ConfigMaps** for adapter configurations (easy updates)

### Operations

11. **Monitor metrics and logs** continuously
12. **Set up alerts** for critical errors and performance issues
13. **Regular health checks** (health endpoints, smoke tests)
14. **Test rollback procedures** before production deployments
15. **Implement CI/CD** for automated testing and deployment

### Security

16. **Use RBAC** with least privilege principle
17. **Rotate secrets regularly** (API tokens, broker credentials)
18. **Use private container registries** for production images
19. **Enable network policies** to restrict traffic
20. **Audit logs** for security events

---

## Directory Structure

```
charts/
├── Chart.yaml                      # Chart metadata
├── values.yaml                     # Default values
├── values-dev.yaml                 # Development overrides
├── values-staging.yaml             # Staging overrides
├── values-prod.yaml                # Production overrides
├── templates/                      # Helm templates
│   ├── _helpers.tpl               # Template helpers
│   ├── configmap-environment.yaml # Environment ConfigMap
│   ├── configmap.yaml             # Adapter config ConfigMaps
│   ├── deployment.yaml            # Deployments
│   ├── hpa.yaml                   # HorizontalPodAutoscaler
│   ├── service.yaml               # Services
│   ├── serviceaccount.yaml        # ServiceAccounts
│   ├── role.yaml                  # ClusterRoles
│   ├── rolebinding.yaml           # ClusterRoleBindings
│   └── servicemonitor.yaml        # Prometheus ServiceMonitors
├── configs/                        # Adapter configurations
│   ├── validation-adapter-config.yaml
│   ├── dns-adapter-config.yaml
│   └── placement-adapter-config.yaml
├── README.md                       # Chart documentation
└── DEPLOYMENT_GUIDE.md             # This file
```

---

## Support

**Documentation:**
- Chart README: `./charts/README.md`
- Adapter Config Template: `../data/adapter-config-template.yaml`
- Contributing Guide: `../CONTRIBUTING.md`

**Troubleshooting:**
- Check logs: `kubectl logs -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter`
- Check events: `kubectl get events -n hyperfleet-system --sort-by='.lastTimestamp'`
- Check health: `kubectl exec -n hyperfleet-system $POD -- curl http://localhost:8081/healthz`

**Community:**
- GitHub: https://github.com/openshift-hyperfleet/hyperfleet-adapter
- Issues: https://github.com/openshift-hyperfleet/hyperfleet-adapter/issues

---

## Appendix: Useful Commands

<details>
<summary>Click to expand: Quick status check</summary>

```bash
# Everything in one command
kubectl get all,cm,secrets,sa,clusterrole,clusterrolebinding,hpa,servicemonitor \
  -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter
```

</details>

<details>
<summary>Click to expand: Debug pod commands</summary>

```bash
# Get shell into running pod
kubectl exec -it -n hyperfleet-system <pod-name> -- sh

# Check environment variables
kubectl exec -n hyperfleet-system <pod-name> -- env | grep -i hyperfleet

# Check mounted files
kubectl exec -n hyperfleet-system <pod-name> -- ls -la /etc/hyperfleet-adapter/
```

</details>

<details>
<summary>Click to expand: Export current configuration</summary>

```bash
# Export all resources
kubectl get all -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter -o yaml > backup.yaml

# Export Helm values
helm get values hyperfleet-adapter -n hyperfleet-system > current-values.yaml
```

</details>

<details>
<summary>Click to expand: Watch resources</summary>

```bash
# Watch pods
watch kubectl get pods -n hyperfleet-system -l app.kubernetes.io/name=hyperfleet-adapter

# Watch HPA
watch kubectl get hpa -n hyperfleet-system

# Watch events
kubectl get events -n hyperfleet-system -w
```

</details>

---

**Last Updated:** 2025-01  
**Chart Version:** See `Chart.yaml`  
**Kubernetes Compatibility:** 1.23+
