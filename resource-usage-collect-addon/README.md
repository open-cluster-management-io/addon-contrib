
# Resource usage collect addon

## Background

With the rapid advancement of artificial intelligence, an increasing number of developers are required to schedule and plan AI/ML workloads based on available resources to achieve optimal performance and resource efficiency.


Open-Cluster-Management (OCM) has already implemented `Placement` and supports [extensible placement scheduling](https://github.com/open-cluster-management-io/enhancements/blob/main/enhancements/sig-architecture/32-extensiblescheduling/32-extensiblescheduling.md), which allows for advanced, customizable workload scheduling across clusters. The key components are:

- `Placement`: This feature enables the dynamic selection of a set of `ManagedClusters` within one or more `ManagedClusterSets` to facilitate Multi-Cluster scheduling.
- `AddOnPlacementScore`: An API introduced by `Placement` to support scheduling based on customized scores.

The `resource-usage-addon` is developed with `AddonTemplate`, and operates within this framework.
- Once installed on the hub cluster, the addon deploys an agent on each managed cluster.
- Agent pods on the managed clusters collect resource usage data and calculate a corresponding score.
- These scores are then used by `Placement` to inform cluster selection, ensuring workloads are deployed on clusters with the most appropriate available resources.

This repository, developed as part of [Google Summer of Code 2024](https://github.com/open-cluster-management-io/ocm/issues/369), introduces enhancements to the `resource-usage-addon`, including new support for scheduling based on GPU and TPU resource availability.
This update is particularly valuable for developers seeking to optimize AI/ML workloads across multiple clusters.


REF:
- [GSoC 2024: Scheduling AI workload among multiple clusters #369](https://github.com/open-cluster-management-io/ocm/issues/369)
- [Extend the multicluster scheduling capabilities with placement](https://open-cluster-management.io/scenarios/extend-multicluster-scheduling-capabilities/)
- [What-is-an-addon](https://open-cluster-management.io/concepts/addon/#what-is-an-add-on)
- [What-is-a-placement](https://open-cluster-management.io/concepts/placement/#select-clusters-in-managedclusterset)
- [Enhancement:addontemplate](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/82-addon-template)

# Quickstart
## Prerequisite
1. Follow the instructions on [OCM official website](https://open-cluster-management.io/getting-started/quick-start/), install `clusteradm` command-line tool and set up a hub (manager) cluster with two managed clusters.
   If prefer using a different kubernetes distribution, follow the instructions in [Set-hub-and-managed-cluster](https://open-cluster-management.io/getting-started/quick-start/#setup-hub-and-managed-cluster).

2. Command line tool `kubectl`  installed.

3. [Docker](https://www.docker.com/) installed.

## Deploy

**Export `kubeconfig` file of your hub cluster.**

```bash
export KUBECONFIG=</path/to/hub_cluster/kubeconfig> # export KUBECONFIG=~/.kube/config
```

**Build the docker image to run the resource-usage-addon.**

```bash
# build image
export IMAGE_NAME=zheshen/resource-usage-collect-addon-template:latest
make images
```

**If you are using kind, load image to your hub cluster.**

```bash
kind load docker-image $IMAGE_NAME --name cluster_name # kind load docker-image  $IMAGE_NAME --name hub
```

**On the hub cluster, deploy the addon.**

```bash
make deploy
```

## What's Next

If deployed successfully:

On the hub cluster, you can see the `AddonTemplate`, and check the `ManagedClusterAddon` status.
```bash
$ kubectl get addontemplate
NAME                     ADDON NAME
resource-usage-collect   resource-usage-collect

$ kubectl get mca -A
NAMESPACE   NAME                     AVAILABLE   DEGRADED   PROGRESSING
cluster1    resource-usage-collect   True                   False
cluster2    resource-usage-collect   True                   False
```

After a short while, on the hub cluster, `AddonPlacementScore` for each managed cluster will be generated.
```bash
$ kubectl config use kind-hub
$ kubectl get addonplacementscore -A
NAMESPACE   NAME                   AGE
cluster1    resource-usage-score   3m23s
cluster2    resource-usage-score   3m24s
```
### Resource Scoring Strategies

#### Node Scope Score
- Node Scope Score: Indicates the available resources on the node with the most capacity in the cluster, aiding in selecting the best node for resource-intensive workloads.
- Code Representation: Represented as `cpuNodeAvailable`, `gpuNodeAvailable`, etc., indicating available CPU and GPU resources on specific nodes.

#### Example Use Scenarios:
- Scenario: Suppose you have a cluster with three nodes: Node A with 2 available GPUs, Node B with 4 available GPUs, and Node C with 6 available GPUs. You need to deploy a job that requires 1 GPU.
- Scheduling Strategies: Using the Node Scope Score, specifically `gpuNodeAvailable`, the scheduler could identify Node A as the optimal node by choosing a lower `gpuNodeAvailable` for this job under a bin-packing strategy. The scheduler would prefer to place the job on Node A to keep Nodes B and C more available for future jobs that may require more resources. This approach minimizes fragmentation and ensures that larger jobs can be accommodated later.

#### Cluster Scope Score
- Cluster Scope Score reflects the total available resources across the entire cluster, helping to determine if the cluster can support additional workloads.
- Code Representation: Represented as `cpuClusterAvailable`, `gpuClusterAvailable`, etc., aggregating available resources across all nodes in the cluster.

#### Example Use Scenarios:
- Scenario: Consider a multi-cluster environment where Cluster X has 10 available GPUs across all nodes, Cluster Y has 6 available GPUs, and Cluster Z has 8 available GPUs. You need to deploy two jobs that first one requires 3 GPUs, and the other requires 4 GPUs.
- Scheduling Strategies: Using the Cluster Scope Score, specifically `gpuClusterAvailable`, the scheduler would prefer the first job Cluster X for the job because it has the most available GPU resource. Then the Cluster X's score becoming lower, the scheduler will then deploy the second job on Cluster Z. This ensures that workloads are spread out, maximizing resource utilization across clusters and avoiding overloading a single cluster.

### Use Placement to select clusters
Consider this example use case: As a developer, I want to select a cluster with the most available GPU resources and deploy a job on it.

Bind the default ManagedClusterSet to default Namespace.
```bash
clusteradm clusterset bind default --namespace default
```
User could create a placement to select one cluster who has the most GPU resources.
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
After the `placement` is created, the user wants to deploy a job to the selected cluster, could use `clusteradm` command to combine these two steps.
```bash
clusteradm create work my-first-work -f work1.yaml --placement default/placement1
````
Then the work will be deployed to the cluster who has been selected. User could see the changes in `addonPlacementScore` if the GPU resources has been consumed by the job.

# Uninstall in the addon.

```bash
# clean up this addon
make undeploy
```

### Troubleshoot
1. If `make deploy` could not work, it might be that there has an auto-generated  `kustomization_tmp.yaml.tmp` file, delete it and rerun the command.
   Also make sure you are under hub cluster context, check the `kustomization.yaml` file, delete the part under `configMapGenerator`(if there is one exists).
