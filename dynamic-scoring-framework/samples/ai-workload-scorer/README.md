# AI Workload Scorer

An external Scoring API for evaluating AI workloads based on GPU power usage and token generation performance.  
It provides two scoring categories — **power** (time-series-based forecasting using [Darts](https://unit8co.github.io/darts/)) and **performance** (static scores from config). The scorer is typically deployed externally and accessed via OpenShift Route.

> For an overview of all available Scoring API samples, see [docs/scoring-api-samples.md](../../docs/scoring-api-samples.md).

## Directory Structure

```
ai-workload-scorer/
├── Dockerfile                # Container image definition
├── README.md                 # This file
├── app/
│   ├── main.py               # FastAPI application entry point
│   ├── schemas/
│   │   ├── config.py         # Pydantic schemas for /config responses
│   │   └── scoring.py        # Pydantic schemas for /scoring request/responses
│   └── static/
│       └── config.json       # Application definitions, device candidates, static scores
├── hack/
│   └── test_scoring.sh       # Script to test the scoring endpoint
├── manifests/
│   └── ai-workload-scorer.yaml  # Kubernetes Deployment/Service manifest
└── static/
    ├── data_power.json       # Sample GPU power time series data for testing
    └── data_performance.json # Sample performance data for testing
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/power/config` | Returns the power scorer configuration |
| `POST` | `/power/scoring` | Accepts GPU power time series data, runs forecasting, and returns power scores |
| `GET` | `/performance/config` | Returns the performance scorer configuration |
| `POST` | `/performance/scoring` | Returns static performance scores from `config.json` |
| `POST` | `/power/reset` | Removes all trained model files to force re-training |
| `GET` | `/healthz` | Health check endpoint |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL_DIR` | `models` | Directory to store trained model files |
| `MIN_LENGTH` | `20` | Minimum number of data points required for prediction |
| `HORIZON_LENGTH` | `10` | Number of future steps to predict |
| `VALIDATION_LENGTH` | `10` | Number of data points used for validation |
| `THRESHOLD` | `200` | Power usage threshold for scoring (sigmoid midpoint) |
| `ALPHA` | `1` | Sigmoid steepness parameter |
| `OFFSET` | `10` | Minimum score offset |
| `MAX_RMSE_THRESHOLD` | `20` | RMSE threshold to trigger model re-training |

## Quick Start (podman)

### Build

```bash
cd samples/ai-workload-scorer
podman build -t ai-workload-scorer .
```

### Run

```bash
podman run -d -p 8000:8000 --name ai-workload-scorer --replace ai-workload-scorer
```

### Test

Check the power configuration:

```bash
curl -sS http://localhost:8000/power/config | jq
```

Check the performance configuration:

```bash
curl -sS http://localhost:8000/performance/config | jq
```

Send sample power scoring data:

```bash
curl -sS -X POST http://localhost:8000/power/scoring \
  -H "Content-Type: application/json" \
  -d @static/data_power.json | jq
```

Get performance scores:

```bash
curl -sS -X POST http://localhost:8000/performance/scoring | jq
```

## How It Works

### Power Scoring (`/power/scoring`)

1. Receives time series data with GPU power usage metrics (DCGM) per pod and node.
2. Pod names are resolved to application names using **glob-style patterns** defined in `app/static/config.json` (`pod_alias`).
3. Per-node total power is computed and a `LinearRegressionModel` (Darts) is trained/loaded for forecasting.
4. Models are automatically re-trained when RMSE exceeds `MAX_RMSE_THRESHOLD`.
5. Per application-device dimension, the estimated power is scored using a sigmoid function:  
   $\text{score} = \left(1 - \frac{1}{1 + e^{-\alpha \cdot (\text{estimated} - \text{threshold})}}\right) \times (100 - \text{offset}) + \text{offset}$
6. A **higher score** means the workload is using **less power** (more headroom); a **lower score** indicates higher power usage.

### Performance Scoring (`/performance/scoring`)

Returns static scores defined in `app/static/config.json` under `static_score.performance`.  
These can represent token generation throughput or other pre-assessed performance metrics.

## Application Configuration (`config.json`)

The `app/static/config.json` file defines:

- **`applications`** — List of applications with `name`, `device_candidates`, and `pod_alias` (glob patterns for pod-to-app mapping).
- **`static_score.performance`** — Static performance scores per app-device combination.

`pod_alias` supports glob-style patterns:
- `*` matches any string (including empty)
- `?` matches any single character
- First match wins (config order)

## Deploy to OpenShift (RHOCP)

```bash
oc create configmap ai-workload-scorer-config \
  --from-file=config.json=app/static/config.json -n ai-workload-scoring
HOST=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
podman login -u ai-ran-admin -p $(oc whoami -t) $HOST
podman build -t $HOST/ai-workload-scoring/ai-workload-scorer:latest .
podman push $HOST/ai-workload-scoring/ai-workload-scorer:latest
```

## Configuration

The scorer exposes the following configurations:

**Power** (`/power/config`):

```json
{
  "name": "ai-workload-power-scorer",
  "description": "A power score for ai workloads based on power usage.",
  "source": {
    "type": "Prometheus",
    "host": "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091",
    "path": "/api/v1/query_range",
    "params": {
      "query": "sum(DCGM_FI_DEV_POWER_USAGE) by (GPU_I_ID, GPU_I_PROFILE, exported_pod, Hostname)",
      "range": 3600,
      "step": 60
    }
  },
  "scoring": {
    "path": "/power/scoring",
    "params": {
      "name": "ai_workload_power_score",
      "interval": 30
    }
  }
}
```

**Performance** (`/performance/config`):

```json
{
  "name": "ai-workload-perf-scorer",
  "description": "A performance score for ai workloads based on token generation rate.",
  "source": {
    "type": "Prometheus",
    "host": "https://thanos-querier.openshift-monitoring.svc.cluster.local:9091",
    "path": "/api/v1/query_range",
    "params": {
      "query": "sum(irate(vllm:generation_tokens_total[1m])) by (pod)",
      "range": 3600,
      "step": 60
    }
  },
  "scoring": {
    "path": "/performance/scoring",
    "params": {
      "name": "ai_workload_perf_score",
      "interval": 30
    }
  }
}
```