# Setup Skupper

Skupper is a tool that enables secure communication between multiple Kubernetes clusters. It is particularly useful in scenarios where services need to communicate across cluster boundaries, such as when the Scoring API is deployed in one cluster and needs to be accessed from other clusters.

## Prerequisites

Before setting up Skupper, ensure you have the following command-line tools installed:

- ```kubectl```
- ```skupper``` **NOTE:** In this setup, we use Skupper v1, not Skupper v2.
- ```podman``` or ```docker```, and ```kind``` if using local clusters.

## Setup Podman Network for Kind Clusters

When using Kind clusters with Podman, each Kind cluster runs in its own isolated network. To enable communication between these clusters using Skupper, you need to create a shared Podman network and connect each Kind cluster's control plane container to this network.

```bash
kubectl create namespace skupper-site-controller --context kind-hub01
kubectl apply -f deploy/skupper/deploy-watch-all-ns.yaml --context kind-hub01
kubectl create namespace skupper-site-controller --context kind-worker01
kubectl apply -f deploy/skupper/deploy-watch-all-ns.yaml --context kind-worker01
kubectl create namespace skupper-site-controller --context kind-worker02
kubectl apply -f deploy/skupper/deploy-watch-all-ns.yaml --context kind-worker02
```

## Configure Podman Network for Kind Clusters

For kind clusters to communicate via Skupper, create a shared podman network:

```bash
podman network create my-kind-net
podman network connect my-kind-net hub01-control-plane
podman network connect my-kind-net worker01-control-plane
podman network connect my-kind-net worker02-control-plane
podman network inspect my-kind-net # Check connected clusters
```

Get the hub cluster IP address.

```bash
HUB_NODE_IP=$(podman inspect hub01-control-plane | jq -r '.[0].NetworkSettings.Networks["my-kind-net"].IPAddress')
echo $HUB_NODE_IP
```

Run the skupper setup script to configure sites and establish connections:

```bash
HUB_NODE_IP=$HUB_NODE_IP ./hack/reset_skupper.sh
```
By default, the script uses the `dynamic-scoring` namespace. To use a different namespace, set the `NAMESPACE` environment variable.

NOTE: the namespace must exist in all clusters before running the script.

Then verify the Skupper setup by checking the sites and connections:

```bash
skupper status -n dynamic-scoring --context kind-hub01
skupper status -n dynamic-scoring --context kind-worker01
skupper status -n dynamic-scoring --context kind-worker02
```
This should show that the sites are connected successfully.

```bash
$ skupper status -n dynamic-scoring --context kind-hub01
Skupper is enabled for namespace "dynamic-scoring" with site name "hub01". It is connected to 2 other sites. It has no exposed services.
```