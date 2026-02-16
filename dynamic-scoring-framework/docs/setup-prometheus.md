# Setup Prometheus

## Setting up Prometheus in Managed Clusters

If you don't have Prometheus set up in your managed clusters, you can deploy a Prometheus instance with kubernetes stack using the following commands:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install kube-prometheus prometheus-community/kube-prometheus-stack --namespace monitoring --create-namespace --kube-context <managed-cluster-context>
```

NOTE: If you want to install Prometheus without kubernetes stack, you can use ```prometheus``` chart instead of ```kube-prometheus-stack``` chart:

```bash
helm install prometheus prometheus-community/prometheus --namespace monitoring --create-namespace --kube-context <managed-cluster-context>
```

When using ```prometheus``` chart, make sure to set up ```ServiceMonitor``` stack as well. This is required for Dynamic Scoring Framework to scrape scoring results from agents. If you don't use score centralization via Prometheus, this step can be skipped.

For checking if Prometheus is running correctly, you can port-forward the Prometheus service and access its web UI:

```bash
export POD_NAME=$(kubectl --namespace monitoring get pod -l "app.kubernetes.io/name=prometheus" -oname --context <managed-cluster-context>)
kubectl --namespace monitoring port-forward $POD_NAME 9090 --context <managed-cluster-context>
```

Then access `http://localhost:9090` in your web browser and query the metrics (e.g. ```container_cpu_usage_seconds_total```).

## Centralized Scoring Results Storage via Prometheus

If you want to send scoring results from managed cluster Prometheus to a centralized Prometheus compatible component in the hub cluster, you can set up Prometheus remote write feature.

### Install VictoriaMetrics on Hub Cluster

To centralize scoring results, VictoriaMetrics is a one of the solutions compatible with Prometheus remote write. You can install VictoriaMetrics on the hub cluster using Helm:

```bash
helm repo add vm https://victoriametrics.github.io/helm-charts/
helm repo update
helm install victoria-metrics vm/victoria-metrics-single -n monitoring --kube-context kind-hub01
```

### Expose VictoriaMetrics via Skupper

If it is necessary to connect to hub VictoriaMetrics from managed clusters, expose the service via Skupper:

```bash
HUB_NODE_IP=$HUB_NODE_IP NAMESPACE=monitoring ./hack/reset_skupper.sh
skupper expose service victoria-metrics-victoria-metrics-single-server --address=vm-hub --port=8428 --namespace monitoring --context kind-hub01
```

### Configure managed cluster Prometheus to Send Metrics to Hub

Update Prometheus on worker clusters to remote-write metrics to the hub's VictoriaMetrics:

```bash
helm upgrade kube-prometheus prometheus-community/kube-prometheus-stack -n monitoring -f deploy/prometheus/values.yaml --kube-context kind-worker01
helm upgrade kube-prometheus prometheus-community/kube-prometheus-stack -n monitoring -f deploy/prometheus/values.yaml --kube-context kind-worker02
```

The `values.yaml` file contains remote_write configuration pointing to `vm-hub:8428` and send scoring results.

If you are using another remote write receiver, modify the `remote_write` section in `values.yaml` accordingly.

### Deploy Recording Rules

Deploy Prometheus recording rules to pre-aggregate metrics. Apply the rules to each managed cluster:

```bash
CLUSTER_NAME=worker01 envsubst < deploy/prometheus/cluster-resource-summary/prometheusrule.yaml | kubectl apply -f - --context kind-worker01
CLUSTER_NAME=worker02 envsubst < deploy/prometheus/cluster-resource-summary/prometheusrule.yaml | kubectl apply -f - --context kind-worker02
```


### Verify Metrics Collection with Grafana

Access Grafana to verify that metrics are being collected:

1. Get the Grafana admin password:

```bash
kubectl get secret --namespace monitoring -l app.kubernetes.io/component=admin-secret -o jsonpath="{.items[0].data.admin-password}" --context kind-hub01 | base64 --decode ; echo
```

2. Set up port forwarding to access Grafana:

```bash
export POD_NAME=$(kubectl --namespace monitoring get pod -l "app.kubernetes.io/name=grafana,app.kubernetes.io/instance=kube-prometheus" -oname --context kind-hub01)
kubectl --namespace monitoring port-forward $POD_NAME 3000 --context kind-hub01
```

3. Open your browser and navigate to `http://localhost:3000`
4. Log in with username `admin` and the password from step 1
5. Add VictoriaMetrics as a data source in Grafana:
   - Go to **Configuration** > **Data Sources** > **Add data source**
   - Select **Prometheus**
   - Set the URL to `http://vm-hub:8428`
   - Click **Save & Test**
6. Create dashboards or use existing ones to visualize metrics from all clusters.

From Grafana, you can verify that metrics from all clusters are being collected in VictoriaMetrics.
