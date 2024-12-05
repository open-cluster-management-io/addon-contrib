# OCM Argo CD Agent Addon

This folder contains resources to deploy and manage the
[Argo CD Agent](https://github.com/argoproj-labs/argocd-agent) within an Open Cluster Management (OCM) ecosystem.
It includes a `charts` directory with three Helm charts that facilitate the setup and management of Argo CD
and its agents across OCM hub and managed clusters.

## Charts Overview

The `charts` folder contains the following Helm charts:

1. **argocd-hub**  
This chart deploys an opinionated Argo CD instance on the OCM hub cluster.
It excludes compute-intensive components (e.g., application controller) to offload workloads to managed clusters.

2. **argocd-addon**  
This chart installs the OCM Argo CD AddOn on the hub cluster.
It automates the deployment of Argo CD instances,
including compute-intensive components, on all managed clusters registered to the hub.

3. **argocd-agent-addon**  
This chart deploys the Argo CD Agent principal component on the hub cluster
and the Argo CD Agent components on all managed clusters.
It enables secure communication, lifecycle management, and application deployment across clusters.

---

Refer to the
[OCM Argo CD Agent Integration Solution](https://github.com/open-cluster-management-io/ocm/tree/main/solutions/deploy-argocd-apps)
documentation for detailed setup and deployment instructions.
