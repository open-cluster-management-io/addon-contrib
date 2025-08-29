package main

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

	"fl_sidecar/exporter"
	"fl_sidecar/watcher"
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

// parseAndPushMtrics parses the content of the metric file and pushes the metrics to the reporter.
func parseAndPushMtrics(reporter *exporter.Reporter, content []byte) {
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
