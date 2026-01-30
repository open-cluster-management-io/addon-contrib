# Install Flower Addon

This guide deploys Flower federated learning infrastructure on Open Cluster Management (OCM).

**What gets deployed:**
- **SuperLink** on hub cluster - Central coordinator for federated learning
- **SuperNode** on managed clusters - Client agents that connect to SuperLink via OCM addon

Both components use official Flower images with `--isolation=process` mode, which separates infrastructure from application code.

## Architecture

```
Hub Cluster                          Managed Clusters
┌─────────────────────────┐          ┌─────────────────────────┐
│  SuperLink (NodePort)   │◄─────────│  SuperNode              │
│  - Fleet API: 30092     │          │  (--isolation=process)  │
│  - Exec API: 30093      │          │  (partition-id=0)       │
│  (--isolation=process)  │          └─────────────────────────┘
└─────────────────────────┘          ┌─────────────────────────┐
┌──────────────────────────┐         │  SuperNode              │
│  OCM Addon               │────────►│  (--isolation=process)  │
│  - AddOnTemplate         │         │  (partition-id=1)       │
│  - ClusterManagementAddon│         └─────────────────────────┘
└──────────────────────────┘
```

## Prerequisites

- Hub cluster with OCM installed
- Managed clusters registered with OCM
- `kubectl`, `helm`, `make`

## Deploy

```bash
make deploy
```

Or manually:

```bash
HUB_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
helm upgrade --install flower-addon charts/flower-addon \
  --set deploymentConfig.superlinkAddress=$HUB_IP
```

Verify:

```bash
kubectl get pods -n flower-system
kubectl get svc -n flower-system
```

## Enable Addon on Clusters

Two options for enabling the addon:

### Option 1: Use defaultConfig from ClusterManagementAddOn

```bash
# Enable addon using the default config (simpler, recommended)
make enable-addon CLUSTER=cluster1
make enable-addon CLUSTER=cluster2
```

### Option 2: Use per-cluster config

```bash
# Configure and enable on cluster1 with specific partition config
make deploy-cluster-config CLUSTER=cluster1 PARTITION_ID=0 NUM_PARTITIONS=2
make enable-addon CLUSTER=cluster1 CONFIG=flower-addon-config

# Configure and enable on cluster2
make deploy-cluster-config CLUSTER=cluster2 PARTITION_ID=1 NUM_PARTITIONS=2
make enable-addon CLUSTER=cluster2 CONFIG=flower-addon-config
```

Or use quick setup:

```bash
make setup-clusters
```

Verify:

```bash
kubectl get managedclusteraddons -A
kubectl --context kind-cluster1 get pods -n flower-addon -l app.kubernetes.io/component=supernode
```

## Cleanup

```bash
make disable-addon CLUSTER=cluster1
make disable-addon CLUSTER=cluster2
make undeploy
```

## Next Steps

- [Run Federated Learning Applications](run-federated-app.md)
- [Auto-Install with Placement](auto-install-by-placement.md)
