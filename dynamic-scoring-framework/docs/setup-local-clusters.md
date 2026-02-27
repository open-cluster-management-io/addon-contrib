# Setup Local Clusters with Kind

This guide provides instructions to set up a local hub cluster and managed clusters using [Kind (Kubernetes IN Docker)](https://kind.sigs.k8s.io/). This setup is useful for development and testing of the Dynamic Scoring Framework.



## Step 1: Install clusteradm CLI

The `clusteradm` CLI tool is required for managing OCM clusters. Install it on your host OS:

```bash
curl -L https://raw.githubusercontent.com/open-cluster-management-io/clusteradm/main/install.sh | bash
```

Verify the installation by checking the version:

```bash
$ clusteradm version
client          version :v1.1.1-0-g802869e
server release  version :v1.32.2
default bundle  version :1.1.1
```

**NOTE**: when using ```--bundle-version=latest``` in the init command, ensure that your `clusteradm` CLI is updated to the latest version to avoid compatibility issues.

## Step 2: Create Kubernetes Clusters

Create three Kubernetes clusters using `kind`:

- **hub01**: The hub cluster that manages worker clusters
- **worker01**: First managed cluster for workload execution
- **worker02**: Second managed cluster for workload execution

```bash
kind create cluster --name hub01
kind create cluster --name worker01
kind create cluster --name worker02
```

## Step 3: Initialize OCM Hub Cluster

Initialize the hub cluster with OCM components. This installs the necessary controllers and CRDs for cluster management:

```bash
clusteradm init --wait --context kind-hub01 --bundle-version=latest
```

This command:

- Installs OCM hub components (cluster-manager)
- Sets up necessary CRDs for managing clusters
- Prepares the hub for accepting managed clusters

## Step 4: Join Worker Clusters to Hub

First, obtain the join token from the hub cluster. This token is used to authenticate worker clusters:

```bash
$ clusteradm get token --context kind-hub01
token=TOKEN
please log on spoke and run:
clusteradm join --hub-token TOKEN --hub-apiserver https://127.0.0.1:36197 --cluster-name <cluster_name>
```

**Note**: Save the `TOKEN` value and API server URL - you'll need them in the next steps.

Now, join each worker cluster using the token. The `--force-internal-endpoint-lookup` flag ensures proper networking in kind clusters:

```bash
clusteradm join --hub-token TOKEN --hub-apiserver https://127.0.0.1:36197 --cluster-name worker01 --context kind-worker01 --force-internal-endpoint-lookup --wait
```

```bash
clusteradm join --hub-token TOKEN --hub-apiserver https://127.0.0.1:36197 --cluster-name worker02 --context kind-worker02 --force-internal-endpoint-lookup --wait
```

**Replace `TOKEN` and the API server URL** with the values from the previous command.

## Step 5: Accept Join Requests

The hub cluster needs to approve the join requests from worker clusters:

```bash
clusteradm accept --context kind-hub01 --clusters worker01,worker02 --wait
```

This creates the necessary resources (ManagedCluster, klusterlet) for each worker cluster.

## Step 6: Verify Cluster Registration

Confirm that the worker clusters are successfully joined and available:

```bash
$ kubectl get managedclusters --all-namespaces --context kind-hub01
NAME       HUB ACCEPTED   MANAGED CLUSTER URLS                  JOINED   AVAILABLE   AGE
worker01   true           https://worker01-control-plane:6443   True     True        150m
worker02   true           https://worker02-control-plane:6443   True     True        148m
```

Both clusters should show:

- **HUB ACCEPTED**: true
- **JOINED**: True
- **AVAILABLE**: True
