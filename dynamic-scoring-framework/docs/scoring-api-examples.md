# Example Scoring APIs

This section demonstrates how to build and deploy various types of Dynamic Scorers. Each scorer showcases different features and deployment patterns:

| Scorer | Location | Input Type | Use Case |
|--------|----------|------------|----------|
| Sample Scorer | Internal (worker01, via NodePort or Skupper) | Time series | Basic CPU-based scoring |
| LLM Forecast Scorer | External (Host machine) | Time series | LLM-powered predictions |
| Simple Prediction Scorer | Internal (worker02, via Skupper) | Time series | Namespace-level forecasts |
| Static Scorer | Internal (worker01, via Skupper) | None | Pre-defined performance/power scores |
| AI Workload Scorer | External (Route) | Time series | AI workload scoring |

## Sample DynamicScorer (Internal, Time Series Input)

This scorer demonstrates a basic time-series-based scoring implementation that runs inside a worker cluster.

Build and tag the image:

```bash
podman build -t sample-scorer samples/sample-scorer
export SAMPLE_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/sample-scorer:latest
podman tag localhost/sample-scorer:latest $SAMPLE_SCORER_IMAGE_NAME
```

Load the image into worker01 and deploy via ManifestWork:

```bash
kind load docker-image  $SAMPLE_SCORER_IMAGE_NAME --name worker01
CLUSTER_NAME=worker01 envsubst < samples/sample-scorer/manifests/manifestwork.yaml | kubectl apply -f - --context kind-hub01
```

Verify the scorer is accessible from the hub cluster.

```bash
kubectl apply -f deploy/utils/test-pod.yaml -n dynamic-scoring --context kind-hub01
# If you are using Skupper, you can access the scorer service directly from each cluster:
kubectl exec -it curl-tester -n dynamic-scoring --context kind-hub01 -- curl -sS http://sample-scorer.dynamic-scoring.svc:8000/config|jq
# If you are using NodePort to access the Scoring API from the hub cluster, use the following command instead:
# kubectl exec -it curl-tester -n dynamic-scoring --context kind-hub01 -- curl -sS http://localhost:30007/config|jq
```

Then query the scorer's configuration endpoint:

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

## External LLM Forecast DynamicScorer (external, Time Series Input)

This scorer demonstrates an external DynamicScorer that uses a Large Language Model (LLM) to forecast time series data.

In this example, the Scoring API receives time series data from the source (Prometheus), sends it to an LLM inference endpoint with some contexts for forecasting, and returns a score based on the forecasted values.

**NOTE**: This example assumes you have a OpenAI-compatible model endpoint running.

build and deploy the external DynamicScorer to the host OS.

```bash
podman build -t llm-forecast-scorer samples/llm-forecast-scorer
podman run -p 8000:8000 --name llm-forecast-scorer --network my-kind-net -e MODEL_NAME=$MODEL_NAME -e INFERENCE_ENDPOINT=$INFERENCE_ENDPOINT --replace -d llm-forecast-scorer
EXTERNAL_SCORER_IP=$(podman inspect llm-forecast-scorer | jq -r '.[0].NetworkSettings.Networks["my-kind-net"].IPAddress')
echo $EXTERNAL_SCORER_IP
curl http://localhost:8000/config|jq
```

External DynamicScorer config:

```json
{
  "name": "llm-forecast-scorer",
  "description": "A sample score for time series data with Inference Endpoint",
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
  
NOTE: The `MODEL_NAME` and `INFERENCE_ENDPOINT` environment variables must be set when running the container.

### Token Authentication with Inference Endpoint

This Scorer is example of using token authentication to access Inference Endpoint.

At first, create a sample API token secret on worker clusters:

```bash
kubectl apply -f secrets/sample-api-token.yaml --context kind-worker01
kubectl apply -f secrets/sample-api-token.yaml --context kind-worker02
```

sample-api-token.yaml:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: api-auth-secret
  namespace: dynamic-scoring
type: Opaque
data:
  token: ZHVtbXktdG9rZW4tMTIzNA==
```

Then, the DynamicScorer CR references this secret for authentication. 

```yaml
  scoring:
    auth:
      tokenSecretRef:
        name: api-auth-secret 
        key: token
```

We can also use token authentication for Source API access by creating a similar secret and referencing it in the `source.auth` section of the DynamicScorer CR.

## Simple Prediction Scorer (internal, Time Series Input)

Prediction scoring is a use case where the DynamicScoring Framework is used to evaluate and score predictions made by various models across multiple clusters. This framework allows for dynamic scoring of these predictions based on metrics collected from Prometheus.

The Simple Prediction Scorer is a scoring API that takes CPU usage as input and predicts CPU usage 5 minutes into the future. It returns a score close to 1 if the predicted value is likely to exceed a predefined threshold.

![alt text](./res/example-simple-prediction-scorer.png)

```bash
podman build -t simple-prediction-scorer samples/simple-prediction-scorer
export SIMPLE_PREDICTION_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/simple-prediction-scorer:latest
podman tag localhost/simple-prediction-scorer:latest $SIMPLE_PREDICTION_SCORER_IMAGE_NAME
kind load docker-image  $SIMPLE_PREDICTION_SCORER_IMAGE_NAME --name worker02
CLUSTER_NAME=worker02 envsubst < samples/simple-prediction-scorer/manifests/manifestwork.yaml | kubectl apply -f - --context kind-hub01
kubectl apply -f tmp/test-pod.yaml --context kind-hub01
kubectl exec -it curl-tester --context kind-hub01 -- curl http://simple-prediction-scorer:8000/config|jq
```

Simple Prediction Scorer config:

```json
{
  "name": "simple-prediction-scorer",
  "description": "A simple prediction score for time series data",
  "source": {
    "type": "Prometheus",
    "host": "http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090",
    "path": "/api/v1/query_range",
    "params": {
      "query": "sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!=\"\", pod!=\"\"}[1m]))",
      "range": 3600,
      "step": 30
    }
  },
  "scoring": {
    "path": "/scoring",
    "params": {
      "name": "simple_prediction_score",
      "interval": 30
    }
  }
}
```



### Static Scorer (internal, no input)

The Static Scorer is a simple implementation of a scoring API that returns pre-defined scores without taking any input. This can be useful when you want to assign fixed scores based on static criteria or use external data sources for scoring.

```bash
podman build -t static-scorer samples/static-scorer
export STATIC_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/static-scorer:latest
podman tag localhost/static-scorer:latest $STATIC_SCORER_IMAGE_NAME
kind load docker-image  $STATIC_SCORER_IMAGE_NAME --name worker01
CLUSTER_NAME=worker01 envsubst < samples/static-scorer/manifestwork.yaml | kubectl apply -f - --context kind-hub01
kubectl apply -f tmp/test-pod.yaml --context kind-hub01
kubectl exec -it curl-tester --context kind-hub01 -- curl http://static-scorer:8000/performance/config|jq
kubectl exec -it curl-tester --context kind-hub01 -- curl http://static-scorer:8000/powerconsumption/config|jq
```

Static Scorer config:

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

### AI Workload Scorer (external, Route)

This scorer demonstrates an example of scoring AI workloads based on GPU power usage. The Scoring API is deployed externally and accessed via OpenShift Route. The source data is collected from Prometheus using a query that sums up the GPU power usage metrics.

This Scoring API provides two scoring endpoints: `/power/scoring` for power usage score and `/performance/scoring` for performance score based on token generation rate. The scores can be used to evaluate the efficiency of AI workloads running in the cluster.

Power Score: It indicates the power headroom of AI workloads. A higher score means the workload is using less power compared to the defined threshold, while a lower score indicates higher power usage.

Performance Score: It indicates the throughput of AI workloads based on token generation rate. A higher score means the workload is generating more tokens, while a lower score indicates less performance.

```bash
podman build -t ai-workload-scorer samples/ai-workload-scorer
export AI_WORKLOAD_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/ai-workload-scorer:latest
podman tag localhost/ai-workload-scorer:latest $AI_WORKLOAD_SCORER_IMAGE_NAME
podman push $AI_WORKLOAD_SCORER_IMAGE_NAME
kubectl apply -f samples/ai-workload-scorer/manifests/ai-workload-scorer.yaml --context kind-hub01
curl http://ai-workload-scorer.cluster-example.com/power/config|jq
```


AI Workload Scorer config:

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

## Register DynamicScorer CRs

Register the DynamicScorer CRs to the hub cluster.

```bash
# Create secrets for Scoring API token authentication
kubectl apply -f secrets/sample-api-token.yaml -n dynamic-scoring --context kind-worker01
kubectl apply -f secrets/sample-api-token.yaml -n dynamic-scoring --context kind-worker02
# Create secrets for Source API token authentication
kubectl apply -f secrets/source-query-secret.yaml -n dynamic-scoring --context kind-worker01
kubectl apply -f secrets/source-query-secret.yaml -n dynamic-scoring --context kind-worker02
# Register DynamicScorer CRs
kubectl apply -f samples/mydynamicscorer-sample.yaml -n open-cluster-management --context kind-hub01
kubectl apply -f samples/mydynamicscorer-external-llm.yaml -n open-cluster-management --context kind-hub01
cat samples/mydynamicscorer-external-llm.yaml | sed "s/\${EXTERNAL_SCORER_IP}/$EXTERNAL_SCORER_IP/g" | kubectl apply -f - -n open-cluster-management --context kind-hub01
kubectl apply -f samples/mydynamicscorer-simple-prediction.yaml -n open-cluster-management --context kind-hub01
kubectl apply -f samples/mydynamicscorer-example-performance.yaml -n open-cluster-management --context kind-hub01
cat samples/mydynamicscorer-example-performance.yaml | sed "s/\${AI_WORKLOAD_SCORER_HOST}/$AI_WORKLOAD_SCORER_HOST/g" | kubectl apply -f - -n open-cluster-management --context kind-hub01
cat samples/mydynamicscorer-example-powerconsumption.yaml | sed "s/\${AI_WORKLOAD_SCORER_HOST}/$AI_WORKLOAD_SCORER_HOST/g" | kubectl apply -f - -n open-cluster-management --context kind-hub01
```