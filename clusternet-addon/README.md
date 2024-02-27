# clusternet-addon

clusternet-addon enable the [clusternet](https://clusternet.io/) proxy on the managed cluster, it helps user to visit
the managed cluster on the hub.

## How to use this addon

1. Deploy clusternet-hub on the OCM hub cluster

2. Deploy the clusternet-addon hub controller on the OCM hub cluster
```
kubectl apply -k deploy/config/manifests
```

3. Create a clusternet ManagedCluster in the OCM managed cluster namespace (e.g. `cluster1`) on the OCM hub cluster
```
cat << EOF | kubectl apply -f -
apiVersion: clusters.clusternet.io/v1beta1
kind: ManagedCluster
metadata:
  labels:
    clusters.clusternet.io/cluster-id: cluster1
  name: clusternet-cluster1
  namespace: cluster1
spec:
  clusterId: 00000000-0000-0000-0000-000000000000
  syncMode: Dual
EOF
```

4. Use clusternet proxy visiting your managed clusters with RBAC, refer to [Visit ManagedCluster With RBAC](https://github.com/clusternet/clusternet/tree/v0.2.0#visit-managedcluster-with-rbac).

**Note**: The direct proxy path is not supported.
