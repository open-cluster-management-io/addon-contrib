# ai-workload-scorer

Sample FastAPI scorer for AI workload performance and power scoring.

## Endpoints

- `POST /performance/scoring`
  - Returns static scores from `app/static/config.json` (`static_score.performance`).
- `POST /power/scoring`
  - Consumes time-series data and computes power scores with a forecasting model.
- `GET /performance/config`
- `GET /power/config`
- `GET /healthz`

## Request schema (power scoring)

`/power/scoring` expects the same structure as other scorers:

```json
{
  "data": [
    {
      "metric": {
        "Hostname": "node01",
        "GPU_I_ID": "0",
        "GPU_I_PROFILE": "3g.48gb",
        "exported_pod": "example-pod"
      },
      "values": [[1700000000, "123.4"], [1700000060, "125.6"]]
    }
  ]
}
```

## config.json pod_alias (glob patterns)

`app/static/config.json` uses **glob-style** patterns in `applications[].pod_alias`:

- `*` matches any string (including empty)
- `?` matches any single character
- Others are literal characters

The **first match wins** (config order). Examples:

- `"worker-decode-0"` (exact match)
- `"app01*"` (prefix match)

## Environment variables

Main options used by the scoring logic:

- `MODEL_DIR` (default: `models`)
- `MIN_LENGTH` (default: `20`)
- `HORIZON_LENGTH` (default: `10`)
- `VALIDATION_LENGTH` (default: `10`)
- `THRESHOLD` (default: `200`)
- `ALPHA` (default: `1`)
- `OFFSET` (default: `10`)
- `MAX_RMSE_THRESHOLD` (default: `20`)

## Local run (podman)

Build:

```bash
podman build -t ai-workload-scorer:latest .
```

Run:

```bash
podman run -d -p 8041:8000 --name ai-workload-scorer --replace ai-workload-scorer:latest
```

Test sample payloads (from `samples/ai-workload-scorer/static`):

```bash
curl -sS -X POST "http://localhost:8041/power/scoring" \
  -H "Content-Type: application/json" \
  -d @samples/ai-workload-scorer/static/data_power.json
```

## Deploy to OpenShift (RHOCP)

```bash
oc create configmap ai-workload-scorer-config --from-file=config.json=app/static/config.json -n ai-workload-scoring
HOST=$(oc get route default-route -n openshift-image-registry --template='{{ .spec.host }}')
podman login -u ai-ran-admin -p $(oc whoami -t) $HOST
podman build -t $HOST/ai-workload-scoring/ai-workload-scorer:latest .
podman push $HOST/ai-workload-scoring/ai-workload-scorer:latest
```