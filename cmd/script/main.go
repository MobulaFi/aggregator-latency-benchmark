package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	fmt.Println("=== Aggregator Indexation Lag Monitor ===")
	fmt.Println("Measuring real-time indexation lag (head lag) for blockchain data APIs")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	config, err := loadEnv()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Metrics will be exposed on :2112/metrics for Prometheus")
	fmt.Println()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Starting Prometheus metrics server on :2112")
		if err := StartMetricsServer(":2112"); err != nil {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	// CoinGecko monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		runGeckoTerminalMonitor(config, stopChan)
	}()

	// Mobula monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMobulaMonitor(config, stopChan)
	}()

	// Codex monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		runCodexMonitor(config, stopChan)
	}()

	// Mobula Pulse V2 monitor (for new pool discovery)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMobulaPulseMonitor(config, stopChan)
	}()

	// Mobula REST API monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMobulaRESTMonitor(config, stopChan)
	}()

	// Codex REST API monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		runCodexRESTMonitor(config, stopChan)
	}()

	<-sigChan
	fmt.Println("\n\nShutting down monitors...")
	close(stopChan)

	wg.Wait()
	fmt.Println("All monitors stopped")
}
