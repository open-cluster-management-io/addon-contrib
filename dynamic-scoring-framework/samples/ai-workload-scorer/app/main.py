from fastapi import FastAPI, Request
import uvicorn
import os
from darts import TimeSeries
from darts.models import LinearRegressionModel
import math
import pandas as pd
import json
import numpy as np
import re
from schemas.config import (
    ConfigResponse,
    PrometheusParams,
    ScoringConfig,
    ScoringParams,
    SourceConfig,
)
from schemas.scoring import Score, ScoringPayload, ScoringResponse


def _glob_to_regex(pattern: str) -> re.Pattern:
    """Convert a glob-like pattern to a compiled regex.

    Supported wildcards:
    - '*' matches any string (including empty)
    - '?' matches any single character
    Rules are always treated as full-string matches.
    """

    if pattern is None:
        return re.compile(r"^$")

    pattern = str(pattern).strip()
    if not pattern:
        return re.compile(r"^$")

    regex = "^" + re.escape(pattern).replace("\\*", ".*").replace("\\?", ".") + "$"
    return re.compile(regex)


MODEL_DIR = os.getenv("MODEL_DIR", "models")
MIN_LENGTH = int(os.getenv("MIN_LENGTH", 20))
HORIZON_LENGTH = int(os.getenv("HORIZON_LENGTH", 10))
VALIDATION_LENGTH = int(os.getenv("VALIDATION_LENGTH", 10))
THRESHOLD = float(os.getenv("THRESHOLD", 200))
ALPHA = float(os.getenv("ALPHA", 1))
OFFSET = float(os.getenv("OFFSET", 10))
MAX_RMSE_THRESHOLD = float(os.getenv("MAX_RMSE_THRESHOLD", 20))
os.makedirs(MODEL_DIR, exist_ok=True)

app = FastAPI()


with open("static/config.json", "r") as f:
    config = json.load(f)
    history_store = {}
    dimentions = [
        (application["name"], device)
        for application in config.get("applications", [])
        for device in application.get("device_candidates", [])
    ]

    # Build a resolver for pod name -> application name.
    # pod_alias is simple glob patterns (strings) only.
    # Priority: first matched rule (in config order).
    application_name_patterns = []  # list[tuple[re.Pattern, str]]

    for application in config.get("applications", []):
        app_name = application.get("name")
        if not app_name:
            continue
        for alias in application.get("pod_alias", []) or []:
            if not alias:
                continue
            try:
                application_name_patterns.append((_glob_to_regex(alias), app_name))
            except re.error:
                continue


def resolve_application_name(pod_name: str) -> str:
    if not pod_name:
        return "unknown"

    # pattern match (includes exact matches because exact strings compile to exact regex)
    for pattern, app_name in application_name_patterns:
        try:
            if pattern.match(pod_name):
                return app_name
        except re.error:
            # Skip invalid regex patterns (shouldn't happen if compiled above)
            continue
    return "unknown"


def resolve_device_profile(device_profile: str) -> str:
    if not device_profile:
        return "unknown"
    elif device_profile.startswith("3g.47gb"):
        return "3g.48gb"
    elif device_profile.startswith("2g"):
        return "2g.24gb"
    elif device_profile.startswith("1g"):
        return "1g.12gb"
    else:
        return "all"


@app.post("/performance/scoring", response_model=ScoringResponse)
async def performance_scoring(request: Request):
    static_scores = config.get("static_score", {}).get("performance", [])
    results = [
        Score(metric=score.get("metric", {}), score=score.get("score", 0))
        for score in static_scores
    ]
    return ScoringResponse(results=results)


@app.post("/power/scoring", response_model=ScoringResponse)
async def power_scoring_timeseries(payload: ScoringPayload, request: Request):
    # print("=== Request Headers ===")
    # for k, v in request.headers.items():
    #     print(f"{k}: {v}")

    data = payload.data
    results = []
    dfs_node = {}

    # Organize data per node and per application-device dimension
    # Build a mapping: node_label -> (app_name, device) -> list of relevant column names
    dim_relevant_cols = {
        node_label: {dim: [] for dim in dimentions}
        for node_label in {
            series.metric.get("Hostname")
            for series in data
            if series.metric.get("Hostname") is not None
        }
    }
    for series in data:
        node_label = series.metric.get("Hostname", "unknown")
        gpu_id = series.metric.get("GPU_I_ID", "0")
        gpu_device = resolve_device_profile(series.metric.get("GPU_I_PROFILE", "all"))
        pod_label = series.metric.get("exported_pod", "unknown")
        app_name = resolve_application_name(pod_label)

        value_label = f"value_{node_label}_GPU{gpu_id}_{gpu_device}_{pod_label}"
        values = [float(v[1]) for v in series.values]
        times = [v[0] for v in series.values]

        # if (app_name, gpu_device) not in dimentions:
        #     continue
        # if len(times) < MIN_LENGTH:
        #     print(f"Skip {value_label} due to insufficient length: {len(times)}")
        #     continue
        if pod_label == "unknown":
            print(f"Skip {value_label} due to unknown pod label.")
            continue

        try:
            df = pd.DataFrame(data={"timestamp": times, value_label: values})
            df["timestamp"] = pd.to_datetime(df["timestamp"], unit="s")
            if node_label not in dfs_node:
                dfs_node[node_label] = df
            else:
                dfs_node[node_label] = pd.merge(
                    dfs_node[node_label],
                    df,
                    on="timestamp",
                    how="outer",
                ).sort_values(by="timestamp")
            if (app_name, gpu_device) in dimentions:
                dim_relevant_cols[node_label][(app_name, gpu_device)].append(
                    value_label
                )
        except Exception as e:
            print(f"Error processing {node_label}: {e}")
            continue

    # Base node-level power prediction
    retrain_flags = {}
    node_base_forecasts = {}
    for node_label, df_node in dfs_node.items():
        df_node["total_power"] = df_node.drop(columns=["timestamp"]).sum(axis=1)

        full_index = pd.date_range(
            start=df_node["timestamp"].min(),
            end=df_node["timestamp"].max(),
            freq="60s",
            name="timestamp",
        )

        df_node = (
            df_node.set_index("timestamp")
            .reindex(full_index)
            .assign(total_power=lambda d: d["total_power"].ffill())
            .reset_index()
        )

        dfs_node[node_label] = df_node

        print(f"Processing node: {node_label}", df_node.shape, df_node.columns)
        print(df_node[["timestamp", "total_power"]].to_string(index=True))
        print(df_node.head())
        if len(df_node) < MIN_LENGTH:
            print(
                f"Skip forecast {node_label} due to insufficient length: {len(df_node)}"
            )
            node_base_forecasts[node_label] = df_node["total_power"].mean()
            continue

        model_path = os.path.join(MODEL_DIR, f"{node_label}.pth")
        if os.path.exists(model_path):
            model = LinearRegressionModel.load(model_path)
            print(f"Loaded model for {node_label} from {model_path}")
            ts = TimeSeries.from_dataframe(
                df_node, "timestamp", "total_power", fill_missing_dates=True, freq="60s"
            )
            train, val = ts[:-HORIZON_LENGTH], ts[-HORIZON_LENGTH:]
            pred = model.predict(n=HORIZON_LENGTH, series=train)
            true_vals = val.values().flatten()
            pred_vals = pred.values().flatten()

            rmse = np.sqrt(((pred_vals - true_vals) ** 2).mean())
            retrain_flags[node_label] = rmse > MAX_RMSE_THRESHOLD
        else:
            retrain_flags[node_label] = True

        if retrain_flags[node_label]:
            print(f"Training/Re-training model for {node_label}")
            ts = TimeSeries.from_dataframe(
                df_node, "timestamp", "total_power", fill_missing_dates=True, freq="60s"
            )
            model = LinearRegressionModel(lags=10)
            try:
                model.fit(ts)
                model.save(model_path)
            except Exception as e:
                print(f"Error training model for {node_label}: {e}")

        forecast = model.predict(n=HORIZON_LENGTH, series=ts)
        forecast_values = forecast.values().flatten()
        mean_val = np.mean(forecast_values)
        node_base_forecasts[node_label] = mean_val

    print("Node base forecasts:", node_base_forecasts)

    cluster_base_forecast = np.sum(list(node_base_forecasts.values()))
    print("Cluster base forecast:", cluster_base_forecast)

    # Individual app-device level power aggregation and scoring
    for dim in dimentions:
        node_histories = []
        for node_label, df_node in dfs_node.items():
            relevant_cols = dim_relevant_cols[node_label][dim]
            if not relevant_cols:
                continue
            df_subset = df_node[["timestamp"] + relevant_cols]
            # Aggregate node-wise app-device power
            node_histories.append(df_subset.drop(columns=["timestamp"]).sum(axis=1))
        if not node_histories:
            print(f"No data for App-Device: {dim}, assigning OFFSET score.")
            normalized_score = OFFSET
        else:
            df_concat = pd.concat(node_histories, axis=1)
            df_concat["timestamp"] = df_node["timestamp"]
            df_concat["app_device_max_power"] = df_concat.drop(
                columns=["timestamp"]
            ).mean(axis=1)
            df_concat = df_concat[["timestamp", "app_device_max_power"]].dropna()
            app_device_max = df_concat["app_device_max_power"].max()
            estimated_score = app_device_max + cluster_base_forecast
            normalized_score = (
                1 - 1 / (1 + math.exp(-ALPHA * (estimated_score - THRESHOLD)))
            ) * (100 - OFFSET) + OFFSET
            print(
                f"App-Device: {dim}, Max Power: {app_device_max}, Estimated Score: {estimated_score}, Normalized Score: {normalized_score}"
            )
        results.append(
            Score(metric={"app": dim[0], "device": dim[1]}, score=normalized_score)
        )

    return ScoringResponse(results=results)


@app.post("/power/reset")
async def reset_prediction_models():
    for model_file in os.listdir(MODEL_DIR):
        if model_file.endswith(".pth"):
            os.remove(os.path.join(MODEL_DIR, model_file))
    return {"status": "ok"}


@app.get("/healthz")
async def healthcheck():
    return {"status": "ok"}


@app.get("/performance/config", response_model=ConfigResponse)
async def get_performance_config():
    return ConfigResponse(
        name="ai-workload-perf-scorer",
        description="A performance score for ai workloads based on token generation rate.",
        source=SourceConfig(
            type="Prometheus",
            host="https://thanos-querier.openshift-monitoring.svc.cluster.local:9091",
            path="/api/v1/query_range",
            params=PrometheusParams(
                query="sum(irate(vllm:generation_tokens_total[1m])) by (pod)",
                range=3600,
                step=60,
            ),
        ),
        scoring=ScoringConfig(
            path="/performance/scoring",
            params=ScoringParams(
                name="ai_workload_perf_score",
                interval=30,
            ),
        ),
    )


@app.get("/power/config", response_model=ConfigResponse)
async def get_power_config():
    return ConfigResponse(
        name="ai-workload-power-scorer",
        description="A power score for ai workloads based on power usage.",
        source=SourceConfig(
            type="Prometheus",
            host="https://thanos-querier.openshift-monitoring.svc.cluster.local:9091",
            path="/api/v1/query_range",
            params=PrometheusParams(
                query="sum(DCGM_FI_DEV_POWER_USAGE) by (GPU_I_ID, GPU_I_PROFILE, exported_pod, Hostname)",
                range=3600,
                step=60,
            ),
        ),
        scoring=ScoringConfig(
            path="/power/scoring",
            params=ScoringParams(
                name="ai_workload_power_score",
                interval=30,
            ),
        ),
    )


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
