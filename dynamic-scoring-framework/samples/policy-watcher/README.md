# Policy Watcher

A lightweight daemon that monitors OCM Policy compliance status on a managed cluster and reflects the result in a `ClusterClaim`.  
It periodically checks all policies in a target namespace and updates a `ClusterClaim` resource with one of three states: `compliant`, `noncompliant`, or `empty`. The claim is only set to `compliant` after policies remain stable for a configurable duration, preventing transient flaps.

> This component is used in the [Optimization Using DSF](../../docs/optimization-using-dsf.md) workflow.

## Directory Structure

```
policy-watcher/
├── Dockerfile          # Container image definition
├── README.md           # This file
├── main.py             # Main daemon loop (Kubernetes client)
├── pyproject.toml      # Python project metadata and dependencies
└── deployment.yaml     # Kubernetes Deployment/RBAC manifest (envsubst-based)
```

## How It Works

1. On startup, connects to the Kubernetes API (in-cluster or via kubeconfig).
2. Every `POLL_INTERVAL_SEC` seconds, lists all OCM `Policy` resources in `WATCH_NAMESPACE`.
3. Determines the aggregate compliance state:
   - **`compliant`** — All policies are `Compliant`.
   - **`noncompliant`** — At least one policy is not `Compliant`.
   - **`empty`** — No policies exist in the namespace.
4. For the `compliant` state, waits `STABLE_DURATION_SEC` seconds of continuous compliance before updating the `ClusterClaim` (debounce).
5. Updates the `ClusterClaim` (`TARGET_CLAIM_NAME`) with the current state.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `POLL_INTERVAL_SEC` | `30` | Polling interval in seconds |
| `STABLE_DURATION_SEC` | `120` | Duration (seconds) policies must remain compliant before claim is updated |
| `TARGET_CLAIM_NAME` | `policy-watcher-claim` | Name of the `ClusterClaim` to update |
| `WATCH_NAMESPACE` | `default` | Namespace to watch for OCM Policy resources |

## Quick Start (podman)

### Build

```bash
cd samples/policy-watcher
podman build -t policy-watcher .
```

### Run (local development with kubeconfig)

```bash
podman run -d \
  --name policy-watcher \
  -v $HOME/.kube/config:/root/.kube/config:ro \
  -e WATCH_NAMESPACE=cluster1 \
  -e POLL_INTERVAL_SEC=10 \
  --replace \
  policy-watcher
```

### Check logs

```bash
podman logs -f policy-watcher
```

## Deploy to Managed Clusters

The `deployment.yaml` uses `envsubst` to inject the cluster name as the watch namespace.

```bash
export CLUSTER_NAME=cluster1
kind load docker-image quay.io/dynamic-scoring/policy-watcher:v0.1.0 --name $CLUSTER_NAME
CLUSTER_NAME=$CLUSTER_NAME envsubst < deployment.yaml | kubectl apply -f - --context kind-$CLUSTER_NAME
```

Repeat for each managed cluster (e.g. `cluster2`).

### Verify

```bash
kubectl get clusterclaims policy-watcher-claim --context kind-cluster1 -o yaml | grep value:
# Expected: value: empty | compliant | noncompliant
```

## RBAC

The `deployment.yaml` includes the required RBAC resources:

- **ClusterRole** — `get`, `list` on `policies.policy.open-cluster-management.io`; `get`, `patch`, `update` on `clusterclaims.cluster.open-cluster-management.io`.
- **ServiceAccount** — `policy-watcher-sa` in `dynamic-scoring` namespace.
- **ClusterRoleBinding** — Binds the above.