# Sample Scorer

A sample Scoring API implementation for the Dynamic Scoring Framework.  
It receives time series data (e.g. CPU usage from Prometheus), processes it, and returns scoring results via a REST API built with FastAPI.

> For an overview of all available Scoring API samples, see [docs/scoring-api-samples.md](../../docs/scoring-api-samples.md).

## Directory Structure

```
sample-scorer/
├── Dockerfile              # Container image definition
├── README.md               # This file
├── app/
│   ├── main.py             # FastAPI application entry point
│   └── schemas/            # Pydantic request/response schemas
├── hack/
│   └── test_scoring.sh     # Script to test the scoring endpoint
├── manifests/
│   ├── manifestwork.yaml   # OCM ManifestWork for deploying to managed clusters
│   └── sample-scorer.yaml  # Kubernetes Deployment/Service manifest
└── static/
    └── data.json           # Sample time series data for testing
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/config` | Returns the scorer configuration (source, scoring params) |
| `POST` | `/scoring` | Accepts time series data and returns scores |
| `GET` | `/healthz` | Health check endpoint |

## Quick Start (podman)

### Build

```bash
cd samples/sample-scorer
podman build -t sample-scorer .
```

### Run

```bash
podman run -d -p 8000:8000 --name sample-scorer --replace sample-scorer
```

### Test

Check the configuration:

```bash
curl -sS http://localhost:8000/config | jq
```

Send sample scoring data:

```bash
curl -sS -X POST http://localhost:8000/scoring \
  -H "Content-Type: application/json" \
  -d @static/data.json | jq
```

Or use the provided test script:

```bash
bash hack/test_scoring.sh
```

## Deploy to Managed Cluster (via OCM ManifestWork)

Load the image into a kind cluster and apply the ManifestWork:

```bash
export SAMPLE_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/sample-scorer:latest
podman tag localhost/sample-scorer:latest $SAMPLE_SCORER_IMAGE_NAME
kind load docker-image $SAMPLE_SCORER_IMAGE_NAME --name cluster1
CLUSTER_NAME=cluster1 envsubst < manifests/manifestwork.yaml | kubectl apply -f - --context kind-hub
```

## Configuration

The scorer exposes the following configuration via `/config`:

```json
{
  "name": "sample-scorer",
  "description": "A sample score for time series data",
  "source": {
    "type": "Prometheus",
    "host": "http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090",
    "path": "/api/v1/query_range",
    "params": {
      "query": "sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!=\"\", pod!=\"\"}[1m]))",
      "range": 3600,
      "step": 60
    }
  },
  "scoring": {
    "path": "/scoring",
    "params": {
      "name": "sample_my_score",
      "interval": 30
    }
  }
}
```