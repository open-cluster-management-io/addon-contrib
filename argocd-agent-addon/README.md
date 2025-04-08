# OCM Argo CD Agent Addon

This folder contains resources to deploy and manage the
[Argo CD Agent](https://github.com/argoproj-labs/argocd-agent) within an Open Cluster Management (OCM) ecosystem.
It includes a `charts` directory with Helm charts that facilitate the setup and management of Argo CD
and its agents across OCM hub and managed clusters.

## Charts Overview

The `charts` folder contains the following Helm charts:

**argocd-addon**  
This chart installs the OCM Argo CD AddOn on the hub cluster.
It automates the deployment of Argo CD instances,
including compute-intensive components, on all managed clusters registered to the hub.

**argocd-agent-addon**  
This chart deploys the Argo CD Agent principal component on the hub cluster
and the Argo CD Agent components on all managed clusters.
It enables secure communication, lifecycle management, and application deployment across clusters.

---

Refer to the
[OCM and Argo CD Agent Integration for Highly Scalable Application Deployment](https://github.com/open-cluster-management-io/ocm/tree/main/solutions/argocd-agent)
documentation for detailed setup and deployment instructions.
