# Configure environment for observability

This guide explains how to configure the observability components to monitor your federated learning environment. To collect metrics from the hub cluster itself, we first need to register it as a managed cluster, a common practice known as `local-cluster`.

## Prerequisites

We assume that you have already installed the environment with 2 managed clusters according to the [Set Up the Environment](../README.md#set-up-the-environment) guide.

Next we will enable local-cluster for observability, because we need to collect metrics from the hub and managed clusters.

## Enable local-cluster

Run the following commands on your hub cluster's control plane to join the hub to itself as a managed cluster.

```bash
# join command
joincmd=$(clusteradm get token --context kind-hub | grep clusteradm)

# join hub cluster as local-cluster
$(echo ${joincmd} --force-internal-endpoint-lookup --wait --context kind-hub | sed "s/<cluster_name>/local-cluster/g")

# accept local-cluster
clusteradm accept --context kind-hub --clusters local-cluster --wait
```

## Verify the environment

Now you can verify that the clusters are registered with the hub:

```bash
kubectl --context kind-hub get managedclusters
```

You should see an output similar to this:

```text
NAME            HUB ACCEPTED   MANAGED CLUSTER URLS                  JOINED   AVAILABLE   AGE
cluster1        true           https://cluster1-control-plane:6443   True     True        3h36m
cluster2        true           https://cluster2-control-plane:6443   True     True        3h36m
local-cluster   true           https://hub-control-plane:6443        True     True        3h35m
```