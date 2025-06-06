# Resource Usage Collect Addon Helm Chart

This Helm chart installs the Resource Usage Collect Addon for Open Cluster Management (OCM). The addon collects resource usage information from managed clusters and reports it to the hub cluster.

## Prerequisites

- Open Cluster Management (OCM) installed
- Helm 3.x
- kubectl configured to access your cluster
- A managed cluster registered with OCM

## Installation

### From Local Chart

```bash
# Clone the repository
git clone https://github.com/open-cluster-management-io/addon-contrib.git
cd addon-contrib/resource-usage-collect-addon/helm

# Install the chart
helm install resource-usage-collect-addon . \
  --namespace open-cluster-management-addon \
  --create-namespace \
  --set global.image.repository=<image-name> # quay.io/haoqing/resource-usage-collect-addon
```

### From Chart Archive

```bash
# Package the chart
helm package .

# Install from the package
helm install resource-usage-collect-addon resource-usage-collect-addon-0.1.0.tgz \
  --namespace open-cluster-management-addon \
  --create-namespace \
  --set global.image.repository=<image-name> # quay.io/haoqing/resource-usage-collect-addon
```

## Configuration

The following table lists the configurable parameters of the Resource Usage Collect Addon chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.image.repository` | Container image repository | `quay.io/open-cluster-management` |
| `global.image.tag` | Container image tag | `latest` |
| `global.image.pullPolicy` | Container image pull policy | `IfNotPresent` |
| `addon.name` | Name of the addon | `resource-usage-collect` |
| `addon.displayName` | Display name of the addon | `Resource Usage Collect` |
| `addon.description` | Description of the addon | `Collects resource usage metrics from managed clusters` |
| `addon.namespace` | Namespace where the addon will be installed | `open-cluster-management-addon` |
| `agent.replicas` | Number of agent replicas | `1` |
| `agent.resources.requests.cpu` | CPU request for the agent | `100m` |
| `agent.resources.requests.memory` | Memory request for the agent | `128Mi` |
| `agent.resources.limits.cpu` | CPU limit for the agent | `500m` |
| `agent.resources.limits.memory` | Memory limit for the agent | `512Mi` |
| `rbac.create` | Whether to create RBAC resources | `true` |
| `skipClusterSetBinding` | skip creating managedclustersetbinding if already exist | `false` |

## Uninstallation

To uninstall/delete the deployment:

```bash
helm uninstall resource-usage-collect-addon -n open-cluster-management-addon
```

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details. 