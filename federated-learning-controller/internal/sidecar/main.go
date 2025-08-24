package main

import (
	"context"
	"fl_sidecar/exporter"
	"fl_sidecar/watcher"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Command-line flags
var (
	metricFile       string
	endpoint         string
	reporterInterval int
	jobName          string
)

func main() {
	// Define and parse command-line flags
	flag.StringVar(&metricFile, "metricfile", "", "Path to the metric file")
	flag.StringVar(&endpoint, "endpoint", "", "Target endpoint address")
	flag.IntVar(&reporterInterval, "interval", 60, "Reporter automatic push interval in seconds")
	flag.StringVar(&jobName, "jobname", "federated-learning-obs-sidecar", "Job name for the metric service")
	flag.Parse()

	// Ensure required flags are provided
	if metricFile == "" || endpoint == "" {
		fmt.Println("Error: -metricfile and -endpoint are required")
		flag.Usage()
		os.Exit(1)
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
	reporter, err := exporter.NewReporter(ctx, endpoint, reporterInterval, jobName)
	if err != nil {
		log.Fatalf("Init metrics reporter failed: %v", err)
	}
	defer reporter.Shutdown(ctx)

	// Initialize the file watcher
	fileWatcher, err := watcher.New(metricFile)
	if err != nil {
		log.Fatalf("Init file watcher failed: %v", err)
	}

	// Start watching the file for updates
	updateChan := fileWatcher.Start(ctx)

	log.Printf("Start watching file %s", metricFile)

	// Main loop to process file updates and handle shutdown
	for {
		select {
		case content, ok := <-updateChan:
			if !ok {
				log.Fatalf("Watcher channel closed")
				return
			}

			// Parse and push metrics in a new goroutine
			go parseAndPushMtrics(reporter, content)

		case <-ctx.Done():
			log.Printf("exiting")
			return
		}
	}
}

// parseAndPushMtrics parses the content of the metric file and pushes the metrics to the reporter.
func parseAndPushMtrics(reporter *exporter.Reporter, content []byte) {
	metrics, labels, err := exporter.ParseContetnt(content)
	if err != nil {
		log.Printf("Parse metrics err: %v", err)
		return
	}

	reporter.UpdateMetrics(metrics, labels)

	// Create a context with a timeout for flushing the metrics
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)

	// Force flush the metrics to the endpoint
	err = reporter.ForceFlush(flushCtx)
	if err != nil {
		log.Printf("Force flush err: %v", err)
	} else {
		for k, v := range metrics {
			log.Printf("Metric: %s, value: %f\n", k, v)
		}
	}

	flushCancel()
}
