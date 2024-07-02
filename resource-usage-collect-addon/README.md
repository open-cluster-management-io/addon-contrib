
# Prototype of extensible scheduling using resources usage.
We already support [extensible placement scheduling](https://github.com/open-cluster-management-io/enhancements/blob/main/enhancements/sig-architecture/32-extensiblescheduling/32-extensiblescheduling.md), which allows use of [addonplacementscore](https://github.com/open-cluster-management-io/enhancements/blob/main/enhancements/sig-architecture/32-extensiblescheduling/32-extensiblescheduling.md#addonplacementscore-api) to select clusters, but we lack an addonplacementscore that contains cluster resource usage information.

In this repo, I developed an addon through addon-freamwork, this addon is mainly used to collect resource usage information on the cluster, and generate an addonplacementscore under the cluster namespace of the hub.

More details refer to [Extend the multicluster scheduling capabilities with placement](https://open-cluster-management.io/scenarios/extend-multicluster-scheduling-capabilities/)

# Quickstart
## Prepare
You have at least two running kubernetes cluster. One is the hub cluster, the other is managedcluster.

You can create an ocm environment by running below command, which will create a hub and two managedclusters for you.

```bash
curl -sSL https://raw.githubusercontent.com/open-cluster-management-io/OCM/main/solutions/setup-dev-environment/local-up.sh | bash
```

## Deploy

Set environment variables.

```bash
export KUBECONFIG=</path/to/hub_cluster/kubeconfig> # export KUBECONFIG=~/.kube/config
```

Build the docker image to run the sample AddOn.

```bash
# build image
export IMAGE_NAME=zheshen/resource-usage-collect-addon-template:latest
make images
```

If you are using kind, load image into kind cluster.

```bash
kind load docker-image $IMAGE_NAME --name cluster_name # kind load docker-image  $IMAGE_NAME --name hub
```

And then deploy the example AddOns controller on hub cluster.

```bash
make deploy
```

On the hub cluster, verify the resource-usage-collect-controller pod is running.
```bash
$ kubectl get pods -n open-cluster-management | grep resource-usage-collect-controller
resource-usage-collect-controller-55c58bbc5-t45dh   1/1     Running   0          71s
```

## What is next

After the deployment is complete, addon will create an addonplacementscore in its own namespace for each managedcluster in the hub.

```bash
$ kubectl config use kind-hub
$ kubectl get addonplacementscore -A
NAMESPACE   NAME                   AGE
cluster1    resource-usage-score   3m23s
cluster2    resource-usage-score   3m24s
```

### For example

Select a cluster with more available GPU.

Bind the default ManagedClusterSet to default Namespace.
```bash
clusteradm clusterset bind default --namespace default
```

```bash
cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: placement1
  namespace: default
spec:
  numberOfClusters: 1
  prioritizerPolicy:
    mode: Exact
    configurations:
      - scoreCoordinate:
          type: AddOn
          addOn:
            resourceName: resource-usage-score
            scoreName: gpuAvailable
        weight: 1
EOF
```

```bash
kubectl get placementdecisions -A
```

# Clean up

```bash
# clean up this addon
make undeploy
```
