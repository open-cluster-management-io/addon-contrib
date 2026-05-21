# Static Scorer

An internal Scoring API that returns pre-defined, static scores without requiring any input data.  
This scorer provides two scoring categories — **performance** and **power consumption** — each returning fixed scores per application and device type. It is useful when you want to assign scores based on static criteria, hardware specifications, or externally managed data sources.

> For an overview of all available Scoring API samples, see [docs/scoring-api-samples.md](../../docs/scoring-api-samples.md).

## Directory Structure

```
static-scorer/
├── Dockerfile              # Container image definition
├── README.md               # This file
├── app/
│   ├── main.py             # FastAPI application entry point
│   └── schemas/
│       ├── config.py       # Pydantic schemas for /config responses
│       └── scoring.py      # Pydantic schemas for /scoring responses
└── manifests/
    └── manifestwork.yaml   # OCM ManifestWork for deploying to managed clusters
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/performance/config` | Returns the performance scorer configuration |
| `POST` | `/performance/scoring` | Returns static performance scores |
| `GET` | `/powerconsumption/config` | Returns the power consumption scorer configuration |
| `POST` | `/powerconsumption/scoring` | Returns static power consumption scores |
| `GET` | `/healthz` | Health check endpoint |

## Quick Start (podman)

### Build

```bash
cd samples/static-scorer
podman build -t static-scorer .
```

### Run

```bash
podman run -d -p 8000:8000 --name static-scorer --replace static-scorer
```

### Test

Check the performance configuration:

```bash
curl -sS http://localhost:8000/performance/config | jq
```

Check the power consumption configuration:

```bash
curl -sS http://localhost:8000/powerconsumption/config | jq
```

Get performance scores:

```bash
curl -sS -X POST http://localhost:8000/performance/scoring | jq
```

Get power consumption scores:

```bash
curl -sS -X POST http://localhost:8000/powerconsumption/scoring | jq
```

## Static Score Values

### Performance Scores

| App | Device | Score |
|-----|--------|-------|
| app01 | all | 80 |
| app02 | all | 80 |
| app01 | 3g.48gb | 60 |
| app02 | 3g.48gb | 55 |
| app01 | 2g.24gb | 10 |
| app02 | 2g.24gb | 15 |

### Power Consumption Scores

| App | Device | Score |
|-----|--------|-------|
| app01 | all | 20 |
| app02 | all | 10 |
| app01 | 3g.48gb | 30 |
| app02 | 3g.48gb | 55 |
| app01 | 2g.24gb | 80 |
| app02 | 2g.24gb | 90 |

> To change the scores, edit the hard-coded values in `app/main.py`.

## Deploy to Managed Cluster (via OCM ManifestWork)

```bash
export STATIC_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/static-scorer:latest
podman tag localhost/static-scorer:latest $STATIC_SCORER_IMAGE_NAME
kind load docker-image $STATIC_SCORER_IMAGE_NAME --name cluster1
CLUSTER_NAME=cluster1 envsubst < manifests/manifestwork.yaml | kubectl apply -f - --context kind-hub
```

## Configuration

The scorer exposes the following configurations:

**Performance** (`/performance/config`):

```json
{
  "name": "example-performance-scorer",
  "description": "An example performance score",
  "source": {
    "type": "None"
  },
  "scoring": {
    "path": "/performance/scoring",
    "params": {
      "name": "example_performance_score",
      "interval": 30
    }
  }
}
```

**Power Consumption** (`/powerconsumption/config`):

```json
{
  "name": "example-powerconsumption-scorer",
  "description": "An example power consumption score",
  "source": {
    "type": "None"
  },
  "scoring": {
    "path": "/powerconsumption/scoring",
    "params": {
      "name": "example_powerconsumption_score",
      "interval": 30
    }
  }
}
```
