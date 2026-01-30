# Auto-Install with Placement

This guide configures automatic addon deployment using OCM Placement API.

**How it works:**
- **Placement** selects clusters by labels (e.g., `gpu=true`) or cluster sets
- **ClusterManagementAddOn** uses `installStrategy: Placements` mode
- OCM automatically creates/removes ManagedClusterAddOn when clusters match/unmatch

This is an alternative to manual `make enable-addon` per cluster.

## Deploy to GPU Clusters

```bash
# Deploy with GPU placement
make deploy-auto-gpu

# Label clusters
kubectl label managedcluster cluster1 gpu=true
kubectl label managedcluster cluster2 gpu=true
```

Verify:

```bash
kubectl get placementdecisions -n open-cluster-management
kubectl get managedclusteraddons -A
```

## Deploy to All Clusters

```bash
make deploy-auto-all
```

## Remove from Auto-Install

Remove label to automatically undeploy:

```bash
kubectl label managedcluster cluster1 gpu-
```

## Switch Back to Manual Mode

```bash
make deploy
```

## Per-Cluster Configuration

By default, `partition-id` is derived from cluster name using a hash function. For explicit control, create per-cluster AddOnDeploymentConfig:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: AddOnDeploymentConfig
metadata:
  name: flower-addon-cluster1
  namespace: cluster1
spec:
  customizedVariables:
    - name: PARTITION_ID
      value: "0"
    - name: NUM_PARTITIONS
      value: "2"
```

Then reference it in ManagedClusterAddOn:

```yaml
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: flower-addon
  namespace: cluster1
spec:
  configs:
    - group: addon.open-cluster-management.io
      resource: addondeploymentconfigs
      name: flower-addon-cluster1
      namespace: cluster1
```

This allows customizing any variable per cluster:
- `PARTITION_ID` - Data partition index
- `NUM_PARTITIONS` - Total number of partitions
- `IMAGE` - Custom SuperNode image
