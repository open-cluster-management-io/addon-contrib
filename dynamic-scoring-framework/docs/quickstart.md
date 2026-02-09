# Dynamic Scoring Framewkork Quickstart

Dynamic Scoring Framework is a framework for automating resource scoring in multi-cluster environments. It calculates dynamic scores for each cluster and provides foundational information for resource optimization and automated control.

## Prepare the Hub Cluster and Managed Clusters

For setting up the hub cluster and managed clusters, please refer to the [Open Cluster Management (OCM) documentation](https://open-cluster-management.io/docs/getting-started/quick-start/).

The typical setup involves:

1. install ```clusteradm``` CLI tool
2. Create a hub cluster and managed clusters if not already available
3. Initialize the hub cluster
4. Join managed clusters to the hub cluster

As an example, local kind clusters can be used for both hub and managed clusters during development. See the [setup local clusters](./setup-local-clusters.md) for more details.

## Setup Dynamic Scoring Framework

You can install the Dynamic Scoring Framework on the hub cluster using the following helm commands:

```bash
helm repo add ocm https://open-cluster-management.io/helm-charts
helm repo update
helm upgrade --install dynamic-scoring-framework ocm/dynamic-scoring-framework --namespace open-cluster-management   --create-namespace
```

```values.yaml``` can be used to customize the installation. For more details, please see the [values.yaml](./../charts/dynamic-scoring-framework/values.yaml) file.

if you want to use customized images for the controllers and agents, please refer to the [advanced configuration section](#advanced-configuration-custom-images) .

## Setup Sample Scoring API and DynamicScorer

### Optional: Setup Skupper

If you want to use Skupper for communication between the hub cluster and managed clusters, please refer to the [Setup Skupper](./setup-skupper.md) guide for instructions on deploying Skupper.

Skupper is required in the following scenarios:

- When the Scoring API is deployed in its own cluster, whether on the hub cluster or on a managed cluster, and needs to be accessed from other clusters.
- When centralized scoring results collection via Prometheus in the hub cluster is used, and the managed clusters need to send scoring results to the hub cluster.

### Optional: Setup Prometheus

**NOTE**: If you don't use Prometheus source in your scoring API or already have Prometheus set up, this step can be skipped.

For Dynamic Scoring Framework to calculate scores from metrics in the managed clusters, Prometheus needs to be set up in each managed cluster.

Please refer to the [Setup Prometheus](./setup-prometheus.md) guide for instructions on deploying Prometheus using Helm.

You can choose to install Prometheus chart based on your use case:

- Just use Prometheus as a metrics store (source) in managed clusters -> ```prometheus``` chart
- use Prometheus as a scoring results store (output) -> ```kube-prometheus-stack``` chart, including ServiceMonitor stack
- use Prometheus for cetralized scoring results store -> ```kube-prometheus-stack``` chart with remote write configuration to the hub cluster

In tne [Setup Prometheus](./setup-prometheus.md) guide, instructions are provided for setting up Prometheus in managed clusters, including optional centralized scoring results collection via Prometheus in the hub cluster.

### Deploy and Register Sample Scoring API

You can register a sample Scoring API by applying the provided sample manifest:

```bash
export SAMPLE_SCORER_IMAGE_NAME=quay.io/dynamic-scoring/sample-scorer:latest
podman build -t $SAMPLE_SCORER_IMAGE_NAME samples/sample-scorer
```

if you want to test Scoring API locally, you can run the sample scorer with the following command:

```bash
podman run -d -p 8000:8000 $SAMPLE_SCORER_IMAGE_NAME
```

Then execute the test script to send a sample scoring request:

```bash
samples/sample-scorer/hack/test_scoring.sh http://localhost:8000 samples/sample-scorer/static/data.json
```

This should return a scoring response with scores using the sample data.

```json
{
  "results": [
    {
      "metric": {
        "__name__": "my_metric_query_name",
        "instance": "localhost:9090",
        "meta": "my_something_meta_by_sample_scorer"
      },
      "score": 10.0
    },
    {
      "metric": {
        "__name__": "my_metric_query_name",
        "instance": "other",
        "meta": "my_something_meta_by_sample_scorer"
      },
      "score": 10.0
    }
  ]
}
```

Apply the Sample Scoring API and DynamicScorer manifest to the hub cluster:

```bash
# if you are using Local kind clusters, load the sample scorer image into the kind cluster
kind load docker-image  $SAMPLE_SCORER_IMAGE_NAME --name worker01
# deploy the sample scorer and register DynamicScorer
CLUSTER_NAME=worker01 envsubst < samples/sample-scorer/manifestwork.yaml | kubectl apply -f - --context kind-hub01
```

The manifestwork deploys the Sample Scoring API in the managed cluster (worker01).

If you already have Skupper set up between the hub and managed clusters, the Sample Scoring API deployed in the managed cluster can be accessed from the hub cluster.

To verify the Sample Scoring API, create a test pod in the hub cluster and exec into it to run the test script:

```bash
kubectl apply -f deploy/utils/test-pod.yaml -n dynamic-scoring --context kind-hub01
kubectl exec -it curl-tester -n dynamic-scoring --context kind-hub01 -- curl -sS http://sample-scorer.dynamic-scoring.svc:8000/config|jq
```

You should see the configuration of the Sample Scoring API.

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

Then, create a DynamicScorer resource to register the Scoring API:

```bash
kubectl apply -f samples/mydynamicscorer-sample.yaml -n open-cluster-management --context kind-hub01
```

**NOTE**: If you install Dynamic Scoring Framework in a different hub namespace, change namespace to create DynamicScorer accordingly. DynamicScorer should be created in the same namespace where Dynamic Scoring Framework is installed.

### Optional: Create Secret for Scoring API Authentication

If you want to use token to access the Scoring API, please create a Secret resource with the token and reference it in the DynamicScorer spec.

For example, you can create a Secret with a sample token in each managed cluster:

```yaml
# secrets/sample-api-token.yaml
apiVersion: v1
kind: Secret
metadata:
  name: api-auth-secret
  namespace: dynamic-scoring
type: Opaque
data:
  token: ZHVtbXktdG9rZW4tMTIzNA== # base64 encoded 'dummy-token-1234'
```

Apply the secret to each managed cluster:


```bash
kubectl apply -f secrets/sample-api-token.yaml -n dynamic-scoring --context kind-worker01
kubectl apply -f secrets/sample-api-token.yaml -n dynamic-scoring --context kind-worker02
```

Then, the DynamicScorer CR references this secret for authentication. 

```yaml
  scoring:
    auth:
      tokenSecretRef:
        name: api-auth-secret 
        key: token
```

**NOTE**: Secret should be created in each managed cluster where the DynamicScoringAgent runs. The namespace should match the namespace where DynamicScoringAgent is deployed (default: dynamic-scoring).

Then, the DynamicScoringAgent will use the token from the Secret to access the Scoring API.

```json
{
  "Authorization": "Bearer dummy-token-1234"
}
```

In your Scoring API implementation, make sure to validate the token from the request header.

## Create DynamicScoringConfig

Apply the DynamicScoringConfig manifest to the hub cluster:

```bash
kubectl apply -f samples/mydynamicscoringconfig.yaml -n open-cluster-management --context kind-hub01
```

After applying, the DynamicScoringConfig controller will create ConfigMaps in each managed cluster.

```bash
$ kubectl get configmap dynamic-scoring-config -n dynamic-scoring --context kind-worker01 -o=jsonpath='{$.data.summaries}'|jq
[
  {
    "name": "sample-scorer",
    "scoreName": "sample_my_score",
    "sourceType": "Prometheus",
    "sourceEndpoint": "http://kube-prometheus-kube-prome-prometheus.monitoring.svc:9090/api/v1/query_range",
    "sourceEndpointAuthName": "",
    "sourceEndpointAuthKey": "",
    "sourceQuery": "sum by (node, namespace, pod) (rate(container_cpu_usage_seconds_total{container!=\"\", pod!=\"\"}[1m]))",
    "sourceRange": 3600,
    "sourceStep": 60,
    "scoringEndpoint": "http://sample-scorer.dynamic-scoring.svc:8000/scoring",
    "scoringInterval": 30,
    "scoringEndpointAuthName": "api-auth-secret",
    "scoringEndpointAuthKey": "token",
    "location": "Internal",
    "scoreDestination": "AddOnPlacementScore",
    "scoreDimensionFormat": "${node};${namespace};${pod}"
  }
]
```

## Check AddOnPlacementScore

You can check the AddOnPlacementScore resources created in the hub cluster:

```bash
$ kubectl get addonplacementscores sample-my-score -n worker01 --context kind-hub01 -o yaml
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: AddOnPlacementScore
metadata:
  creationTimestamp: "2026-02-05T11:44:16Z"
  generation: 1
  name: sample-my-score
  namespace: worker01
  resourceVersion: "327413"
  uid: 17676d46-569c-496f-9d56-e87354b385fc
status:
  scores:
  - name: worker01-control-plane;open-cluster-management-agent;klusterlet-work-agent-6477c8c976-bk9nt
    value: 0
  - ...
```

The AddonPlacementScore is named using ```scoreName```, and its namespace corresponds to the managed cluster name.
The scores are listed in the status section. and the ```scores[].name``` are formatted based on the ```scoreDimensionFormat``` specified in the DynamicScorer.

**NOTE**: The AddOnPlacementScore supports integer scores. If the Scoring API returns floating-point scores, they will be rounded down to the nearest integer by the DynamicScoringAgent.

## Optional: Query Scoring Results from Prometheus

If you have set up Prometheus in the managed clusters, you can query the scoring results exported by the DynamicScoringAgent. For scraping the scoring results, make sure to deploy the provided ServiceMonitor manifest in each managed cluster:

```bash
kubectl apply -f deploy/agentfeedback -n dynamic-scoring --context kind-worker01
kubectl apply -f deploy/agentfeedback -n dynamic-scoring --context kind-worker02
```

If you have set up centralized scoring results collection via Prometheus in the hub cluster (refer to the [centralized scoring results collection section](./setup-prometheus.md#centralized-scoring-results-collection)), you can query the scoring results from the hub's Prometheus.

For example, to query the sample score:

```bash
kubectl exec -it --context kind-hub01 curl-tester -n dynamic-scoring -- curl http://vm-hub.monitoring.svc:8428/api/v1/query?query="avg(dynamic_score\{ds_score_name=\"sample_my_score\"\})by(ds_cluster)"|jq
```

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "ds_cluster": "worker01"
        },
        "value": [
          1770604907,
          "0.014498105493169442"
        ]
      },
      {
        "metric": {
          "ds_cluster": "worker02"
        },
        "value": [
          1770604907,
          "0.015842464788628705"
        ]
      }
    ]
  },
  "stats": {
    "seriesFetched": "49",
    "executionTimeMsec": 1
  }
}
```


## Next Steps

- Install on OpenShift: [OpenShift Installation Guide](./install-on-ocp.md)
- Scoring API Examples: [Scoring API Samples](./scoring-api-samples.md)

## Advanced Configuration: custom images

TBD
