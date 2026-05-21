from fastapi import FastAPI, HTTPException
from typing import List, Optional
import requests
import uvicorn
import re
import os
from jinja2 import Environment, FileSystemLoader, TemplateNotFound
from schemas.config import (
    ConfigResponse,
    PrometheusParams,
    ScoringConfig,
    ScoringParams,
    SourceConfig,
)
from schemas.scoring import Score, ScoringPayload, ScoringResponse

app = FastAPI(title="Time Series Forecasting with LLM Inference")

INFERENCE_ENDPOINT = os.getenv(
    "INFERENCE_ENDPOINT", "http://localhost:8000/v1/completions"
)
MODEL_NAME = os.getenv("MODEL_NAME", "Qwen/Qwen2.5-14B-Instruct")

# Try to load a Jinja2 template from templates/request_evaluation.j2. If not
# present, fall back to the simple build_prompt() function below.
env = Environment(loader=FileSystemLoader("templates"))
try:
    template = env.get_template("request_evaluation.j2")
except TemplateNotFound:
    template = None


def build_prompt(
    timestamps: List[str], values: List[float], metadata: Optional[str]
) -> str:
    time_series_str = "\n".join([f"{ts}, {val}" for ts, val in zip(timestamps, values)])
    prompt = (
        "Given the following time series data, predict the **next single numeric value only**.\n"
        "Respond with only the single predicted float number, no other text.\n"
    )
    if metadata:
        prompt += f"Additional context: [{metadata}]\n"
    prompt += f"Time Series: [\n{time_series_str}]\nNext value prediction:"
    return prompt


def run_inference(prompt: str) -> float:
    payload = {
        "model": MODEL_NAME,
        "prompt": prompt,
        "temperature": 0.7,
        "max_tokens": 64,
    }
    response = requests.post(INFERENCE_ENDPOINT, json=payload)
    if response.status_code != 200:
        raise HTTPException(status_code=500, detail=response.text)

    content = response.json()["choices"][0]["text"]
    # print(f"Model response: {content.strip()}")

    # Parse out the numeric prediction from the model response
    match = re.search(r"-?\d+(\.\d+)?", content)
    if not match:
        raise HTTPException(
            status_code=500,
            detail=f"Failed to extract numeric prediction from model response: prompt={prompt}, response={content}",
        )
    return float(match.group(0))


@app.post("/scoring", response_model=ScoringResponse)
async def scoring_timeseries(payload: ScoringPayload):
    data = payload.data
    results = []
    for series in data:
        timestamp = [int(v[0]) for v in series.values]
        values = [float(v[1]) for v in series.values]
        metric = series.metric
        app_name = metric.get("app", "unknown")
        with open("./contexts/tendency.txt", "r") as f:
            tendency = f.read().strip()

        context = (
            f"timeseries_kind: APP_CPU_LOAD, app_name: {app_name}, tendency: {tendency}"
        )

        # If a template is available, render the prompt using the template for
        # more flexible prompt authoring. Otherwise fall back to build_prompt().
        if template:
            try:
                timeseries = list(zip(timestamp, values))
                prompt = template.render(context=context, timeseries=timeseries)
            except Exception:
                # If template rendering fails for any reason, fall back to the
                # previous behavior to avoid blocking scoring.
                prompt = build_prompt(timestamp, values, context)
        else:
            prompt = build_prompt(timestamp, values, context)
        # print(f"Generated prompt: {prompt}")
        try:
            prediction = run_inference(prompt)
            results.append(Score(metric=series.metric, score=prediction))
        except Exception as e:
            print(f"Error during inference: {str(e)}")
            results.append(Score(metric=series.metric, score=0))
    return ScoringResponse(results=results)


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.get("/config", response_model=ConfigResponse)
async def get_config():
    return ConfigResponse(
        name="llm-forecast-scorer",
        description="A sample score for time series data with LLM-based forecasting",
        source=SourceConfig(
            type="Prometheus",
            host="http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090",
            path="/api/v1/query_range",
            params=PrometheusParams(
                query='sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))',
                range=3600,
                step=60,
            ),
        ),
        scoring=ScoringConfig(
            path="/scoring",
            params=ScoringParams(
                name="llm_forecast_score",
                interval=60,
            ),
        ),
    )


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
