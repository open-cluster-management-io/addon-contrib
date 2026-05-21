# LLM Forecast Scorer

An external Scoring API that uses a Large Language Model (LLM) to forecast time series data and return scores.  
The scorer receives time series data (e.g. CPU usage from Prometheus), constructs a prompt with contextual information, sends it to an OpenAI-compatible inference endpoint, and returns a score based on the predicted next value.

> For an overview of all available Scoring API samples, see [docs/scoring-api-samples.md](../../docs/scoring-api-samples.md).

## Directory Structure

```
llm-forecast-scorer/
├── Dockerfile                  # Container image definition
├── README.md                   # This file
├── app/
│   ├── main.py                 # FastAPI application entry point
│   ├── llm-test.py             # Standalone script for testing LLM inference
│   ├── contexts/
│   │   └── tendency.txt        # Additional context fed into the LLM prompt
│   ├── schemas/
│   │   ├── config.py           # Pydantic schemas for /config response
│   │   └── scoring.py          # Pydantic schemas for /scoring request/response
│   └── templates/
│       └── request_evaluation.j2  # Jinja2 template for LLM prompt generation
├── hack/
│   └── test_scoring.sh         # Script to test the scoring endpoint
└── static/
    └── data.json               # Sample time series data for testing
```

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/config` | Returns the scorer configuration (source, scoring params) |
| `POST` | `/scoring` | Accepts time series data, runs LLM inference, and returns scores |
| `GET` | `/healthz` | Health check endpoint |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `INFERENCE_ENDPOINT` | `http://localhost:8000/v1/completions` | OpenAI-compatible completions endpoint URL |
| `MODEL_NAME` | `Qwen/Qwen2.5-14B-Instruct` | Model name to use for inference |

## Quick Start (podman)

### Prerequisites

An OpenAI-compatible model endpoint must be running and accessible from the container.

### Build

```bash
cd samples/llm-forecast-scorer
podman build -t llm-forecast-scorer .
```

### Run

```bash
export MODEL_NAME="Qwen/Qwen2.5-14B-Instruct"
export INFERENCE_ENDPOINT="http://<your-inference-host>:8000/v1/completions"

podman run -d -p 8000:8000 \
  --name llm-forecast-scorer \
  -e MODEL_NAME=$MODEL_NAME \
  -e INFERENCE_ENDPOINT=$INFERENCE_ENDPOINT \
  --replace \
  llm-forecast-scorer
```

> **Tip**: If you are running the scorer alongside kind clusters, add `--network my-kind-net` so that the container can communicate with them.

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
bash hack/test_scoring.sh http://localhost:8000 static/data.json
```

## How It Works

1. The `/scoring` endpoint receives time series data (timestamp + value pairs per metric).
2. For each series, a prompt is built using:
   - The Jinja2 template (`templates/request_evaluation.j2`) if available, otherwise a built-in fallback function.
   - Additional context from `contexts/tendency.txt` describing data characteristics.
3. The prompt is sent to the configured LLM inference endpoint.
4. The numeric prediction is extracted from the model response and returned as the score.

## Prompt Customization

You can customize the LLM prompt by editing `app/templates/request_evaluation.j2`.  
The template receives the following variables:

- `context` — A string containing metadata (e.g. `timeseries_kind`, `app_name`, `tendency`).
- `timeseries` — A list of `(timestamp, value)` tuples.

You can also modify `app/contexts/tendency.txt` to provide different contextual hints to the model.

## Configuration

The scorer exposes the following configuration via `/config`:

```json
{
  "name": "llm-forecast-scorer",
  "description": "A sample score for time series data with LLM-based forecasting",
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
      "name": "llm_forecast_score",
      "interval": 60
    }
  }
}
```

## Token Authentication

This scorer supports token authentication for accessing the inference endpoint.  
See the [Token Authentication](../../docs/scoring-api-samples.md#token-authentication-with-inference-endpoint) section in the scoring API samples documentation for details.
