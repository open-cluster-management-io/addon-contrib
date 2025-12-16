from fastapi import FastAPI, Request
from pydantic import BaseModel
from typing import List, Dict
import uvicorn
import os
import torch
from darts import TimeSeries
from darts.models import LinearRegressionModel
import math
import pandas as pd
import json
import base64
import numpy as np

MODEL_DIR = os.getenv("MODEL_DIR", "models")
MIN_LENGTH = int(os.getenv("MIN_LENGTH", 20))
HORIZON_LENGTH = int(os.getenv("HORIZON_LENGTH", 10))
VALIDATION_LENGTH = int(os.getenv("VALIDATION_LENGTH", 10))
THRESHOLD = float(os.getenv("THRESHOLD", 1))
ALPHA = float(os.getenv("ALPHA", 20))
MAX_RMSE_THRESHOLD = float(os.getenv("MAX_RMSE_THRESHOLD", 20))
os.makedirs(MODEL_DIR, exist_ok=True)

app = FastAPI()


class TimeSeriesSample(BaseModel):
    metric: Dict[str, str]
    values: List[List[float]]  # e.g., [[timestamp, value], ...]


class ScoringPayload(BaseModel):
    data: List[TimeSeriesSample]


@app.post("/scoring")
async def scoring_timeseries(payload: ScoringPayload, request: Request):
    print("=== Request Headers ===")
    for k, v in request.headers.items():
        print(f"{k}: {v}")

    data = payload.data
    results = []
    for series in data:
        predictor_label = f"{series.metric.get('node', 'unknown')}_{series.metric.get('namespace', 'unknown')}_{series.metric.get('pod', 'unknown')}"
        values = [float(v[1]) for v in series.values]
        times = [v[0] for v in series.values]

        if len(times) < MIN_LENGTH:
            print(f"Skip {predictor_label} due to insufficient length: {len(times)}")
            continue

        try:
            df = pd.DataFrame(data={"timestamp": times, "value": values})
            df["timestamp"] = pd.to_datetime(df["timestamp"], unit="s")
            ts = TimeSeries.from_dataframe(df, "timestamp", "value")
        except Exception as e:
            print(f"Error processing {predictor_label}: {e}")
            continue

        model_path = os.path.join(MODEL_DIR, f"{predictor_label}.pth")
        if os.path.exists(model_path):
            # 学習済みモデルのロード
            model = LinearRegressionModel.load(model_path)
            print(f"Loaded model for {predictor_label} from {model_path}")
            train, val = ts[:-HORIZON_LENGTH], ts[-HORIZON_LENGTH:]
            pred = model.predict(n=HORIZON_LENGTH, series=train)
            true_vals = val.values().flatten()
            pred_vals = pred.values().flatten()

            rmse = np.sqrt(((pred_vals - true_vals) ** 2).mean())
            if rmse > MAX_RMSE_THRESHOLD:  # 例: MAX_RMSE_THRESHOLD = 20.0
                print(f"Re-training model for {predictor_label} due to high RMSE")
                model = LinearRegressionModel(lags=10)
                model.fit(ts)
                model.save(model_path)
        else:
            # 新規モデル作成、学習
            model = LinearRegressionModel(lags=10)
            model.fit(ts)
            model.save(model_path)
            print(f"Trained and saved new model for {predictor_label} at {model_path}")

        # 10ステップ先までの予測
        forecast = model.predict(n=HORIZON_LENGTH, series=ts)
        forecast_values = forecast.values().flatten()
        max_val = max(forecast_values)
        ratio_over = 1 / (1 + math.exp(-ALPHA * (max_val - THRESHOLD))) * 100

        metadata = {
            "predictor_label": predictor_label,
            "alpha": ALPHA,
            "threshold": THRESHOLD,
        }
        metadata_str = json.dumps(metadata, separators=(",", ":"), ensure_ascii=False)
        metadata_str_encoded = base64.b64encode(metadata_str.encode("utf-8")).decode(
            "utf-8"
        )
        series.metric["meta"] = metadata_str_encoded

        results.append({"metric": series.metric, "score": ratio_over})
    return {"results": results}


@app.post("/reset")
async def reset_prediction_models():
    for model_file in os.listdir(MODEL_DIR):
        if model_file.endswith(".pth"):
            os.remove(os.path.join(MODEL_DIR, model_file))
    return {"status": "ok"}


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.get("/config")
async def get_config():
    config = {
        "name": "simple-prediction-scorer",
        "description": "A simple prediction score for time series data",
        "source": {
            "type": "prometheus",
            "host": "http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090",
            "path": "/api/v1/query_range",
            "params": {
                "query": 'sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!="", pod!=""}[1m]))',
                "range": 3600,
                "step": 30,
            },
        },
        "scoring": {
            "path": "/scoring",
            "params": {
                "name": "simple_prediction_score",
                "interval": 30,
            },
        },
    }
    return config


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
