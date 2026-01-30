# Run Federated Learning Applications

This guide builds and runs FL applications on OCM federation using process isolation mode. OCM automatically distributes ClientApp jobs to all managed clusters with the flower-addon enabled - no manual per-cluster deployment needed.

**What gets deployed:**
- **SuperExec-ServerApp** on hub - Executes ServerApp (aggregation logic)
- **SuperExec-ClientApp** on managed clusters - Executes ClientApp (local training)

Both use a single `flower-app` image containing your ML code and dependencies (PyTorch, etc.). This separates application code from infrastructure (SuperLink/SuperNode use official images).

**Deployment method:**
- ServerApp: Direct deployment to hub cluster
- ClientApp: ManifestWorkReplicaSet distributes to clusters with flower-addon enabled

## Architecture

```
Hub Cluster                          Managed Clusters
┌─────────────────────────┐          ┌──────────────────────────────────────┐
│  flower-system          │          │  flower-addon                        │
│  ├── SuperLink          │◄─────────│  ├── SuperNode                       │
│  │   (official image)   │          │  │   (official image)                │
│  └── SuperExec-ServerApp│          │  └── SuperExec-ClientApp             │
│      (flower-app image) │          │      (flower-app image)              │
└─────────────────────────┘          └──────────────────────────────────────┘
```

## Prerequisites

- Infrastructure deployed: Complete [Install Flower Addon](install-flower-addon.md) first
- At least 2 SuperNodes connected
- ManifestWorkReplicaSet feature enabled (for ClientApp distribution):
  ```bash
  kubectl patch clustermanager cluster-manager --type=merge \
    -p '{"spec":{"workConfiguration":{"featureGates":[{"feature":"ManifestWorkReplicaSet","mode":"Enable"}]}}}'
  ```

## Build and Deploy App

```bash
# Build app image
make build-app

# Load into Kind clusters
make load-app-kind

# Deploy SuperExec components
make deploy-app
```

Verify:

```bash
# ServerApp on hub
kubectl get pods -n flower-system -l app.kubernetes.io/component=superexec-serverapp

# ClientApp on managed clusters (deployed by ManifestWorkReplicaSet)
kubectl --context kind-cluster1 get pods -n flower-addon -l app.kubernetes.io/component=superexec-clientapp
```

## Run FL Training

```bash
# Setup Python environment
uv venv && uv pip install -e .

# Submit FL app
make run-app
```

## Monitor

```bash
# ServerApp logs
kubectl logs -n flower-system -l app.kubernetes.io/component=superexec-serverapp -f

# ClientApp logs
kubectl --context kind-cluster1 logs -n flower-addon -l app.kubernetes.io/component=superexec-clientapp -f
```

## Custom Image

Build with custom registry:

```bash
APP_IMAGE=quay.io/your-org/flower-app:1.0.0 make build-app
APP_IMAGE=quay.io/your-org/flower-app:1.0.0 make push-app
```

Update `cifar10/deploy/kustomization.yaml`:

```yaml
images:
  - name: flower-app
    newName: quay.io/your-org/flower-app
    newTag: "1.0.0"
```

Redeploy:

```bash
make deploy-app
```

## Cleanup

```bash
make undeploy-app
```
