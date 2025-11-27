# OpenTelemetry Collector Addon for Open Cluster Management

## Overview

The OpenTelemetry Collector Addon is a pluggable addon for Open Cluster Management (OCM) that automates the deployment and management of OpenTelemetry collector on managed clusters. Built on the [addon-framework](https://github.com/open-cluster-management-io/addon-framework), it provides observability and metrics collection capabilities across your multi-cluster environment.

### Key Features

- 🚀 **Automated Deployment**: One-click installation of OpenTelemetry collector across all managed clusters
- 🔐 **Secure by Default**: Automatic TLS certificate generation and management
- 📊 **Prometheus Integration**: Built-in Prometheus stack with remote write receiver
- ⚙️ **Configurable**: Flexible configuration options for different environments
- 🎯 **Cluster Targeting**: Smart placement decisions based on cluster labels
- 📈 **Metrics Collection**: Collects Kubernetes and container metrics from managed clusters

### Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Hub Cluster   │    │ Managed Cluster  │    │ Managed Cluster │
│                 │    │                  │    │                 │
│ ┌─────────────┐ │    │ ┌──────────────┐ │    │ ┌──────────────┐ │
│ │ Prometheus  │ │◄───┤ │ OTel         │ │    │ │ OTel         │ │
│ │ (Remote     │ │    │ │ Collector    │ │    │ │ Collector    │ │
│ │ Write)      │ │    │ │              │ │    │ │              │ │
│ └─────────────┘ │    │ └──────────────┘ │    │ └──────────────┘ │
│                 │    │        │         │    │        │         │
│ ┌─────────────┐ │    │ ┌──────▼──────┐  │    │ ┌──────▼──────┐  │
│ │ OTel Addon  │ │    │ │ Node Metrics│  │    │ │ Node Metrics│  │
│ │ Manager     │ │    │ │ cAdvisor    │  │    │ │ cAdvisor    │  │
│ └─────────────┘ │    │ └─────────────┘  │    │ └─────────────┘  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Quick Start

### Prerequisites

- Open Cluster Management (OCM) environment ([Quick Start Guide](https://open-cluster-management.io/docs/getting-started/quick-start/))
- Helm 3.x installed
- kubectl configured to access your hub cluster

### Installation (Recommended - Using Makefile)

Install the OpenTelemetry addon with automatic Prometheus stack and certificate generation:

```bash
# Clone the repository
git clone https://github.com/open-cluster-management-io/addon-contrib.git
cd addon-contrib/open-telemetry-addon

# Install everything (certificates, prometheus, addon)
make install-all

# Or install step by step
make generate-certs
make install-prometheus
make install-addon
```

That's it! The Makefile will automatically:
- Generate TLS certificates for secure communication
- Deploy Prometheus with remote write receiver and TLS configuration
- Deploy OpenTelemetry collector to all eligible managed clusters
- Start collecting metrics from Kubernetes nodes and containers

### Non-TLS Installation (For Existing Prometheus)

If you already have Prometheus installed and don't need TLS:

```bash
# Using Makefile (recommended)
make install-addon-no-tls

# Or using Helm directly
helm install open-telemetry-addon ./charts/open-telemetry-addon \
  --namespace open-cluster-management-addon \
  --create-namespace \
  --set opentelemetryCollector.tls.enabled=false
```

## Configuration

### Helm Chart Configuration

The addon can be configured through Helm values. See [charts/open-telemetry-addon/values.yaml](charts/open-telemetry-addon/values.yaml) for all available options.

#### TLS Configuration

```yaml
# Enable/disable TLS for Prometheus communication
opentelemetryCollector:
  tls:
    enabled: true  # Set to false for non-TLS Prometheus endpoints
```

#### Common configurations:

```yaml
# Custom Prometheus endpoint
addonDeploymentConfig:
  customVariables:
    PROM_REMOTE_WRITE_ENDPOINT: "https://your-prometheus:9090/api/v1/write"

# Custom certificate settings
certificates:
  server:
    commonName: "your-prometheus-server"
    altNames:
      - "your-prometheus-server"
      - "prometheus.your-domain.com"

# Enable Grafana dashboard
kube-prometheus-stack:
  grafana:
    enabled: true
    service:
      type: NodePort
      nodePort: 30080
```

## Verification

### Check Addon Status

```bash
# Verify addon installation
kubectl get clustermanagementaddon otel

# Check placement decisions
kubectl get placementdecisions -n open-cluster-management-addon

# View addon template
kubectl get addontemplate otel-addon -o yaml
```

### Verify collector on Managed Clusters

```bash
# Check collector deployment (replace cluster1 with your cluster)
kubectl --context kind-cluster1 get pods -n open-cluster-management-agent-addon

# Expected output:
# NAME                                     READY   STATUS    RESTARTS   AGE
# opentelemetry-collector-xxxx-xxxx        1/1     Running   0          2m
```

### Query Metrics

Access Prometheus and query collected metrics:

```bash
# Port forward to Prometheus (if using NodePort, access directly)
kubectl port-forward -n monitoring svc/prometheus-stack-kube-prom-prometheus 9090:9090 &

# Query container metrics from managed clusters
curl -k "https://localhost:9090/api/v1/query?query=container_fs_inodes_total" | jq .

# Expected: Metrics with cluster_name labels from all managed clusters
```

Example metrics output:
```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "container_fs_inodes_total",
          "cluster_name": "cluster1",
          "instance": "cluster1-control-plane",
          "job": "opentelemetry-collector"
        },
        "value": [1751596235.174, "104840504"]
      }
    ]
  }
}
```

### Health Checks

The OpenTelemetry collector include built-in health checks:

```bash
# Check collector health on managed cluster
kubectl --context kind-cluster1 exec -n open-cluster-management-agent-addon \
  deployment/opentelemetry-collector -- \
  curl http://localhost:13133/
```

## Troubleshooting

### Common Issues

1. **Collector not starting**
   ```bash
   # Check collector logs
   kubectl --context kind-cluster1 logs -n open-cluster-management-agent-addon deployment/opentelemetry-collector
   
   # Check if Prometheus endpoint is accessible
   kubectl --context kind-cluster1 exec -n open-cluster-management-agent-addon deployment/opentelemetry-collector -- \
     curl -k https://hub-control-plane:30090/api/v1/write
   ```

2. **No metrics appearing**
   ```bash
   # Verify placement decision
   kubectl get placementdecisions -n open-cluster-management-addon -o yaml
   
   # Check cluster labels
   kubectl get managedcluster --show-labels
   ```

3. **Certificate issues**
   ```bash
   # Check if secrets exist
   kubectl get secret prometheus-tls -n open-cluster-management-addon
   kubectl get secret otel-signer -n open-cluster-management-hub
   ```
