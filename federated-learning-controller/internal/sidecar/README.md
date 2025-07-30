# FL sidecar

A sidecar utility to get metrics from a file and forward them to an OpenTelemetry collector.

## How to Use

Run the FL Sidecar from the command line, must specify the path to the metric file and the endpoint for the OpenTelemetry collector.

```bash
go run main.go -metricfile <path-to-metric-file> -endpoint <otlp-grpc-endpoint>
```

### Command-line Arguments

  * `-metricfile` (required): The file path for the metrics that the sidecar should watch.
  * `-endpoint` (required): The OTLP/gRPC endpoint of the OpenTelemetry collector (e.g., `localhost:4317`).
  * `-interval` (optional): The interval in seconds for the reporter to automatically push metrics. The default is 60 seconds.

### Metric File Format

The sidecar expects the metric file to be in JSON format. The file should contain a single JSON object where keys are the metric names (strings) and values are the metric values (numbers).

**NOTE:** When updating the metrics each time, it will automatically add the `timestamp` field to the metrics. But if the `metric.json` file already contains a `timestamp` field, it will overwrite the value with the current timestamp.

**Example:**

```json
{
  "epoch": 1,
  "loss": 0.543,
  "accuracy": 0.87
}
```

The sidecar watches this file for changes and sends the key-value pairs as metrics to the collector.

## Run as a Container

**Build the Image**

Run the following command to build the Docker image.

```bash
make build-app-image
```

**Push the Image**

Before pushing, make sure to update the `REGISTRY` variable in the `Makefile` to your own container registry (e.g., `docker.io/username`).

```bash
make push-app-image
```

**Run the Container**

Use the `docker run` command to start the sidecar, must mount the metric file's directory into the container and provide the necessary arguments.

```bash
docker run --rm \
  -v /path/on/host/to/metrics:/app/metrics \
  docker-registry/fl-sidecar:latest \
  -metricfile /app/metrics/fl_metrics.txt \
  -endpoint host.docker.internal:4317
```

**Explanation:**

  * `--rm`: Automatically removes the container when it exits.
  * `-v /path/on/host/to/metrics:/app/metrics`: This mounts the directory containing metric file from host machine into the `/app/metrics` directory inside the container. **This is required** for the sidecar to access the file.
  * `-metricfile /app/metrics/fl_metrics.txt`: Tells the sidecar where to find the metric file *inside the container*.
  * `-endpoint host.docker.internal:4317`: Specifies the OTLP collector endpoint.