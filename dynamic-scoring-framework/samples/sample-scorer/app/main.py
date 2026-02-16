from fastapi import FastAPI, Request
from schemas.scoring import ScoringPayload, ScoringResponse
from schemas.config import ConfigResponse

import uvicorn

app = FastAPI()


@app.post("/scoring", response_model=ScoringResponse)
async def scoring_timeseries(payload: ScoringPayload, request: Request):
    print("=== Request Headers ===")
    for k, v in request.headers.items():
        print(f"{k}: {v}")

    data = payload.data
    results = []
    for series in data:
        series.metric["meta"] = "my_something_meta_by_sample_scorer"
        values = [float(v[1]) for v in series.values]
        # Compute a simple average as the score
        average = sum(values) / len(values) if values else 0
        results.append({"metric": series.metric, "score": average})
    return {"results": results}


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.get("/config", response_model=ConfigResponse)
async def get_config():
    config = {
        "name": "sample-scorer",
        "description": "A sample score for time series data",
        "source": {
            "type": "Prometheus",
            "host": "http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090",
            "path": "/api/v1/query_range",
            "params": {
                "query": 'sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))',
                "range": 3600,
                "step": 60,
            },
        },
        "scoring": {
            "path": "/scoring",
            "params": {
                "name": "sample_my_score",
                "interval": 30,
            },
        },
    }
    return config


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
