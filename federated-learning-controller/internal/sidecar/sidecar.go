package sidecar

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github/open-cluster-management/federated-learning/internal/sidecar/exporter"
	"github/open-cluster-management/federated-learning/internal/sidecar/watcher"
)

// Config holds the configuration for the sidecar
type Config struct {
	MetricFile       string
	Endpoint         string
	ReporterInterval int
	JobName          string
}

// Run starts the sidecar with the given configuration
func Run(cfg *Config) error {
	// Ensure required config is provided
	if cfg.MetricFile == "" || cfg.Endpoint == "" {
		return fmt.Errorf("metricfile and endpoint are required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a channel to listen for OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("exiting")
		cancel()
	}()

	// Initialize the metrics reporter
	reporter, err := exporter.NewReporter(ctx, cfg.Endpoint, cfg.ReporterInterval, cfg.JobName)
	if err != nil {
		return fmt.Errorf("init metrics reporter failed: %w", err)
	}
	defer reporter.Shutdown(ctx)

	// Initialize the file watcher
	fileWatcher, err := watcher.New(cfg.MetricFile)
	if err != nil {
		return fmt.Errorf("init file watcher failed: %w", err)
	}

	// Start watching the file for updates
	updateChan := fileWatcher.Start(ctx)

	log.Printf("Start watching file %s", cfg.MetricFile)

	// Check if main container process is still running periodically
	go func() {
		for {
			// Check if main container is still running (when shareProcessNamespace is enabled)
			if checkMainContainerExited() {
				log.Println("Main container exited, exiting sidecar...")
				cancel()
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
				// Check every 5 seconds
				time.Sleep(5 * time.Second)
			}
		}
	}()

	// Main loop to process file updates and handle shutdown
	for {
		select {
		case content, ok := <-updateChan:
			if !ok {
				log.Println("Watcher channel closed")
				return nil
			}

			// Parse and push metrics in a new goroutine
			go parseAndPushMetrics(reporter, content)

		case <-ctx.Done():
			log.Printf("exiting")
			return nil
		}
	}
}

// ParseFlags parses command-line flags and returns a Config
func ParseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.MetricFile, "metricfile", "", "Path to the metric file")
	flag.StringVar(&cfg.Endpoint, "endpoint", "", "Target endpoint address")
	flag.IntVar(&cfg.ReporterInterval, "interval", 60, "Reporter automatic push interval in seconds")
	flag.StringVar(&cfg.JobName, "jobname", "federated-learning-obs-sidecar", "Job name for the metric service")
	return cfg
}

// checkMainContainerExited checks if the main container (flower-server or flower-client) process has exited
// This works when shareProcessNamespace is enabled in the pod spec
func checkMainContainerExited() bool {
	// Check if the main flower server or client process is still running
	// Try different patterns for server and client containers
	patterns := []string{
		".*server.*--num-rounds",     // Server process pattern
		".*client.*--data-config",    // Client process pattern
		".*client.*--server-address", // Alternative client pattern
	}

	for _, pattern := range patterns {
		cmd := exec.Command("pgrep", "-f", pattern)
		err := cmd.Run()
		if err == nil {
			// Found a matching process, main container is still running
			return false
		}
	}

	// No matching processes found, main container has likely exited
	return true
}

// parseAndPushMetrics parses the content of the metric file and pushes the metrics to the reporter.
func parseAndPushMetrics(reporter *exporter.Reporter, content []byte) {
	metrics, labels, err := exporter.ParseContetnt(content)
	if err != nil {
		log.Printf("Parse metrics err: %v", err)
		return
	}

	reporter.UpdateMetrics(metrics, labels)

	// Force flush the metrics to the endpoint
	err = reporter.ForceFlush()
	if err != nil {
		log.Printf("Force flush err: %v", err)
	}
}
