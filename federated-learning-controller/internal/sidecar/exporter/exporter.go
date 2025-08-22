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

// Reporter is responsible for exporting metrics to an OTLP endpoint.
type Reporter struct {
	// provider is the OTLP metric provider.
	provider *sdkmetric.MeterProvider
	// meter is the OTLP meter.
	meter metric.Meter
	// mu is a mutex to protect the metrics map.
	mu sync.Mutex
	// metrics is a map of metric names to values.
	metrics map[string]float64
	// gauges is a map of metric names to OTLP gauges.
	gauges map[string]metric.Float64ObservableGauge
	// callbackRegistration is the registration for the metric callback.
	callbackRegistration metric.Registration
	// round is the current round number.
	round *int
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
		metrics:  make(map[string]float64),
		gauges:   make(map[string]metric.Float64ObservableGauge),
	}

	log.Println("Init metric reporter success")
	return r, nil
}

// UpdateMetrics updates the metrics in the reporter.
func (r *Reporter) UpdateMetrics(newMetrics map[string]float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.metrics = newMetrics

	// Overwrite timestamp with current time
	if _, ok := r.metrics["timestamp"]; ok {
		log.Println("overwrite timestamp")
	}
	r.metrics["timestamp"] = float64(time.Now().Unix())

	// Extract round from metrics, if round not present, set it to nil
	if roundFloat, ok := r.metrics["round"]; ok {
		log.Println("metric has round field")
		rountInt := int(roundFloat)
		r.round = &rountInt
		delete(r.metrics, "round")
	} else {
		log.Println("metric has no round field")
		r.round = nil
	}

	// Check if the metric set has changed
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

	// If the metric set has not changed, do nothing
	if !needsUpdate {
		return
	}

	log.Println("Metric set changed, re-registering callback")

	// Unregister the old callback
	if r.callbackRegistration != nil {
		if err := r.callbackRegistration.Unregister(); err != nil {
			log.Printf("Error unregistering callback: %v", err)
		}
	}

	// Create new gauges for the new metrics
	r.gauges = make(map[string]metric.Float64ObservableGauge, len(r.metrics))
	instruments := make([]metric.Observable, 0, len(r.metrics))

	for name := range r.metrics {
		gauge, err := r.meter.Float64ObservableGauge(name)
		if err != nil {
			log.Printf("Error creating gauge for %s: %v", name, err)
			continue
		}
		r.gauges[name] = gauge
		instruments = append(instruments, gauge)
	}

	if len(instruments) == 0 {
		return
	}

	// Register a new callback to observe the gauges
	registration, err := r.meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			for name, gauge := range r.gauges {
				if value, ok := r.metrics[name]; ok {
					var roundLabel metric.MeasurementOption
					if r.round == nil {
						roundLabel = metric.WithAttributes(attribute.String("round", "nil"))
					} else {
						roundLabel = metric.WithAttributes(attribute.Int("round", *r.round))
					}
					o.ObserveFloat64(gauge, value, roundLabel)
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
func (r *Reporter) ForceFlush(ctx context.Context) error {
	return r.provider.ForceFlush(ctx)
}
