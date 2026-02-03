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

	// Always scrape a fresh Defined.fi session cookie at startup
	fmt.Println("Scraping fresh Defined.fi session cookie...")
	sessionCookie, err := RefreshSessionCookie()
	if err != nil {
		fmt.Printf("Warning: Failed to scrape session cookie: %v\n", err)
		fmt.Println("Codex REST monitor may not work properly")
		// Try to use existing cookie if available
		if config.DefinedSessionCookie != "" {
			fmt.Println("Falling back to existing DEFINED_SESSION_COOKIE from environment")
		}
	} else {
		config.DefinedSessionCookie = sessionCookie
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

	// Quote API latency monitor (Jupiter, Li.Fi, 1inch, KyberSwap)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runQuoteAPIMonitor(config, stopChan)
	}()

	// Metadata coverage monitor (Mobula vs Codex)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMetadataCoverageMonitor(config, stopChan)
	}()

	// Head lag monitor (blockchain head vs aggregator indexed head)
	wg.Add(1)
	go func() {
		defer wg.Done()
		runHeadLagMonitor(config, stopChan)
	}()

	<-sigChan
	fmt.Println("\n\nShutting down monitors...")
	close(stopChan)

	wg.Wait()
	fmt.Println("All monitors stopped")
}
