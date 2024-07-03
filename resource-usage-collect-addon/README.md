
# Resource usage addon
## Background
Open-Cluster-Management has already supported [extensible placement scheduling](https://github.com/open-cluster-management-io/enhancements/blob/main/enhancements/sig-architecture/32-extensiblescheduling/32-extensiblescheduling.md), which allow users to use [addonplacementscore](https://github.com/open-cluster-management-io/enhancements/blob/main/enhancements/sig-architecture/32-extensiblescheduling/32-extensiblescheduling.md#addonplacementscore-api) to select clusters under certain conditions.

The basic idea of `addonPlacementScore` is that, the addon agent, which is installed on the managed cluster, collect information about the managed cluster, and calculate a score. These scores can be used when selecting or comparing multiple clusters.
With the rapid advancement of artificial intelligence, an increasing number of developers need to schedule and plan workloads based on available resources to achieve better performance and save resources.

This repository mainly introduce a addon who collect the resource usage information in the managed clusters and calculate `addonPlacementScore`, users could select clusters based on the score using a `placement`.
A possible use case could be: As a developer, I want to deploy my work on the cluster who has the most GPU resources available. This addon is developed using `addonTemplate`.

More details about:
- Extensible scheduling, please refer to [Extend the multicluster scheduling capabilities with placement](https://open-cluster-management.io/scenarios/extend-multicluster-scheduling-capabilities/)
- Add-on, please refer to [What-is-an-addon](https://open-cluster-management.io/concepts/addon/#what-is-an-add-on)
- Placement, please refer to [What-is-a-placement](https://open-cluster-management.io/concepts/placement/#select-clusters-in-managedclusterset)
- Addon template, please refer to [Enhancement:addontemplate](https://github.com/open-cluster-management-io/enhancements/tree/main/enhancements/sig-architecture/82-addon-template)

# Quickstart
## Prerequisite
1. Follow the instructions on [OCM official website](https://open-cluster-management.io/getting-started/quick-start/) install`clusteradm` command-line tool and set up a hub (manager) cluster and two managed clusters. 
If using a different kubernetes distribution, follow the instructions in [Set-hub-and-managed-cluster](https://open-cluster-management.io/getting-started/quick-start/#setup-hub-and-managed-cluster).
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

On the hub cluster, you can see the `addonTemplate`, and check the `managedClusterAddon` status.
```bash
$ kubectl get addontemplate
NAME                     ADDON NAME
resource-usage-collect   resource-usage-collect

$ kubectl get mca -A
NAMESPACE   NAME                     AVAILABLE   DEGRADED   PROGRESSING
cluster1    resource-usage-collect   True                   False
cluster2    resource-usage-collect   True                   False
```

After a short while,on the hub cluster, `addonPlacementScore` for each managed cluster will be generated.
```bash
$ kubectl config use kind-hub
$ kubectl get addonplacementscore -A
NAMESPACE   NAME                   AGE
cluster1    resource-usage-score   3m23s
cluster2    resource-usage-score   3m24s
```

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
Also make sure you are under hub cluster context, check the `kustomization.yaml` file, delete the part under `configMapGenerator`.
