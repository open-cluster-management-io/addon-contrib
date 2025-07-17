# Kueue Addon for Open Cluster Management

This addon provides an integration between Kueue and Open Cluster Management (OCM) to simplify MultiKueue setup and enhance multicluster scheduling capabilities.

This integration is designed for Kueue users who want to leverage OCM's capabilities to improve their multicluster workload scheduling experience. 

The Kueue addon offers two primary benefits:

- **Simplified MultiKueue Setup**
   - Automates the generation of MultiKueue specific Kubeconfig.
   - Streamlines the configuration of MultiKueue resources.

- **Enhanced Multicluster Scheduling**
   - Integrates with OCM's placement with MultiKueue by implementing an admission check controller.
   - Generate MultiKueueConfig & MultiKueueCluster dynamically based on OCM placement decision.


## Description

This repository contains the kueue addon controller and addon chart to deploy resources to hub and spoke clusters.

### Architecture Overview

![Architecture Diagram](./arch.png)

### Kueue addon controller
This controller is running on the hub, contains a credential controller and an admission check controller.
- **Credential Controller**
    - Create `ClusterPermission` and `ManagedServiceAccount` for each spoke to get credential.
    - Generates kubeconfig secrets for MultiKueue under `kueue-system` namespace.
- **Admission Check Controller**
    - Watches the `Placement` and `PlacementDecision` to generates `MultiKueueConfig` and `MultiKueueCluster` resources dynamically.
    - Set the `AdmissionCheck` condition `Active` to true when success.

### Addon chart
- **Addon deployment:** Deploy [Kueue addon controllers](#kueue-addon-controller) on the hub.
- **Addon Template:** To deploy `ResourceFlavor`, `ClusterQueue` and `LocalQueue` resources need by MultiKueue to spoke clusters.
- **Other addon files:**  `ClusterManagementAddOn`, `ClusterRole`, `ClusterRoleBinding`, `ManagedClusterSetBinding`, `Placement` etc. 

## Prerequisites

- Open Cluster Management (OCM) installed with the following addons:
  - [Cluster Permission Addon](https://github.com/open-cluster-management-io/cluster-permission)
  - [Managed Service Account Addon](https://github.com/open-cluster-management-io/managed-serviceaccount)
- Kueue already installed on the hub and spoke cluster
- MultiKueue enabled on the hub

## Quick Start

**_Important_**: The addon requires you already installed OCM, Cluster Permission Addon, Managed Service Account Addon and Kueue. 
The whole setup steps about this solution, please refer to this [Kueue Integration Solution](https://github.com/open-cluster-management-io/ocm/blob/main/solutions/kueue-admission-check).

## Installation


On the hub cluster, deploy the addon.

```bash
make deploy
```

You can install the addons via the helm charts.

```bash
helm install kueue-addon charts/kueue-addon/ \
  -n open-cluster-management-addon --create-namespace
```

To confirm the installation from hub:

```bash
$ kubectl get cma kueue-addon
NAME          DISPLAY NAME   CRD NAME
kueue-addon   kueue-addon    

$ kubectl get mca -A | grep kueue-addon
NAMESPACE          NAME                     AVAILABLE   DEGRADED   PROGRESSING
<your cluster>     kueue-addon              True                   False

$ kubectl get deploy -n open-cluster-management  kueue-addon-controller
NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
kueue-addon-controller   1/1     1            1           4h27m
```

To confirm the installation from spoke:

```bash
$ kubectl get clusterqueue
NAME            COHORT   PENDING WORKLOADS
cluster-queue            0

$ kubectl get localqueue
NAME         CLUSTERQUEUE    PENDING WORKLOADS   ADMITTED WORKLOADS
user-queue   cluster-queue   0                   0

$ kubectl get resourceflavor
NAME             AGE
default-flavor   4h28m
```

## Usage

The usage please refer to this [Kueue Integration Solution](https://github.com/open-cluster-management-io/ocm/blob/main/solutions/kueue-admission-check).