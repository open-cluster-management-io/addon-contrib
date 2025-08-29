package exporter

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// Reporter is responsible for managing and exporting application metrics
// to an OTLP (OpenTelemetry Protocol) endpoint. It encapsulates the creation,
// registration, and update of metric instruments and their values.
//
// The Reporter is safe for concurrent use.
type Reporter struct {
	// provider is the OTLP metric provider.
	// It owns the lifecycle of metric collection and export.
	provider *sdkmetric.MeterProvider

	// meter is used to create and register metric instruments (gauges, counters, etc).
	meter metric.Meter

	// mu protects access to the internal state (metrics, labels, gauges, callback).
	mu sync.Mutex

	// gauges holds the mapping of metric names to their corresponding
	// Float64ObservableGauge instruments.
	// Only updated when the metric set changes (add/remove).
	gauges map[string]metric.Float64ObservableGauge

	// callbackRegistration is the handle returned by RegisterCallback.
	// It allows unregistering the callback when the metric set changes.
	callbackRegistration metric.Registration

	// metrics stores the latest values for each metric (name -> value).
	// These values are read inside the callback at collection time.
	metrics map[string]float64

	// labels stores the latest global labels (key -> float64 value),
	// applied to all observed metrics during collection.
	labels map[string]float64
}

// NewReporter creates a new Reporter instance.
func NewReporter(ctx context.Context, endpoint string, interval int, jobName string) (*Reporter, error) {
	// Create a new OTLP metric exporter
	exporter, err := otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(endpoint),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Get pod name and namespace from environment variables
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")

	// Create a new resource with service name, namespace, and pod name attributes
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(jobName),
			attribute.Key("pod.namespace").String(namespace),
			attribute.Key("pod.name").String(podName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create a new meter provider with the resource and periodic reader
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Duration(interval)*time.Second))),
	)

	// Create a new meter
	meter := meterProvider.Meter("fl-train-events")

	// Create a new Reporter
	r := &Reporter{
		provider: meterProvider,
		meter:    meter,
	}

	log.Println("Init metric reporter success")
	return r, nil
}

// UpdateMetrics updates the set of observable metrics in the reporter.
// If the set of metrics has changed (new metric names added/removed),
// it will re-register the callback with the new gauges.
// Otherwise, it simply updates the values to be observed.
//
// newMetrics: map of metric name -> current value
// newLabels:  map of label key -> float64 value (labels applied to all metrics)
func (r *Reporter) UpdateMetrics(newMetrics map[string]float64, newLabels map[string]float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update the metrics and labels
	r.metrics = newMetrics
	r.labels = newLabels

	// Always overwrite "timestamp" metric with the current Unix time.
	// This ensures we have a fresh timestamp regardless of input.
	r.metrics["timestamp"] = float64(time.Now().Unix())

	// Step 1: Detect if the set of metrics has changed
	// (different number of metrics or different names)
	needsUpdate := false
	if len(r.metrics) != len(r.gauges) {
		needsUpdate = true
	} else {
		for name := range r.metrics {
			if _, ok := r.gauges[name]; !ok {
				needsUpdate = true
				break
			}
		}
	}

	// If the metric set is unchanged, just return.
	// Values will be observed in the callback during collection.
	if !needsUpdate {
		return
	}

	log.Println("Metric set changed, re-registering callback")

	// Step 2: Unregister the old callback if it exists
	if r.callbackRegistration != nil {
		if err := r.callbackRegistration.Unregister(); err != nil {
			log.Printf("Warning: failed to unregister old callback: %v", err)
		}
		r.callbackRegistration = nil
	}

	// Step 3: Create new gauges for all metrics
	r.gauges = make(map[string]metric.Float64ObservableGauge, len(r.metrics))
	instruments := make([]metric.Observable, 0, len(r.metrics))

	for name := range r.metrics {
		gauge, err := r.meter.Float64ObservableGauge(name)
		if err != nil {
			log.Printf("Error creating gauge for %q: %v", name, err)
			continue
		}
		r.gauges[name] = gauge
		instruments = append(instruments, gauge)
	}

	if len(instruments) == 0 {
		log.Println("No valid instruments created; callback not registered")
		return
	}

	// Step 4: Register a new callback to observe the gauges
	registration, err := r.meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			r.mu.Lock()
			defer r.mu.Unlock()

			for name, gauge := range r.gauges {
				if value, ok := r.metrics[name]; ok {
					// Convert newLabels into ObserveOptions
					labels := make([]metric.ObserveOption, 0, len(r.labels))
					for k, v := range r.labels {
						labels = append(labels, metric.WithAttributes(attribute.Float64(k, v)))
					}
					log.Printf("Observing %q: %f", name, value)
					o.ObserveFloat64(gauge, value, labels...)
				}
			}
			return nil
		},
		instruments...,
	)
	if err != nil {
		log.Printf("Error registering callback: %v", err)
		return
	}

	r.callbackRegistration = registration
}

// Shutdown shuts down the reporter.
func (r *Reporter) Shutdown(ctx context.Context) error {
	return r.provider.Shutdown(ctx)
}

// ForceFlush forces the reporter to flush all buffered metrics.
func (r *Reporter) ForceFlush() error {
	// Create a context with a timeout for flushing the metrics
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()

	// Force flush the metrics to the endpoint
	err := r.provider.ForceFlush(flushCtx)
	if err != nil {
		log.Printf("Force flush err: %v", err)
	}
	return err
}
