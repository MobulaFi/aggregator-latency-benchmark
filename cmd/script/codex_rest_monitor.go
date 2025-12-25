package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	codexRESTBaseURL = "https://graph.codex.io/graphql"
)

// Chains for REST monitoring
var codexRESTChains = []struct {
	networkID   int
	chainName   string
	poolAddress string
}{
	{1399811149, "solana", "7qbRF6YsyGuLUVs6Y1q64bdVrfe4ZcUUz1JRdoVNUJnm"},
	{56, "bnb", "0x58f876857a02d6762e0101bb5c46a8c1ed44dc16"},
	{8453, "base", "0x4c36388be6f416a29c8d8eee81c771ce6be14b18"},
	{143, "monad", "0x659bD0BC4167BA25c62E05656F78043E7eD4a9da"},
}

type CodexGraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type CodexGraphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// callCodexGraphQLAPI makes a GraphQL query to Codex API
func callCodexGraphQLAPI(apiKey string, poolAddress string, networkID int, chainName string) (float64, int, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Build GraphQL query - get pool bar data (OHLCV) for realistic latency measurement
	// This fetches actual market data that isn't cached, giving us real API performance
	now := time.Now().Unix()
	from := now - 3600 // Last hour

	query := `
		query GetPoolBars($address: String!, $networkId: Int!, $from: Int!, $to: Int!) {
			getBars(
				symbol: ""
				address: $address
				networkId: $networkId
				resolution: "1"
				from: $from
				to: $to
				currencyCode: "USD"
			) {
				t
				o
				h
				l
				c
				v
			}
		}
	`

	// Build request body with variables
	reqBody := CodexGraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"address":   poolAddress,
			"networkId": networkID,
			"from":      int(from),
			"to":        int(now),
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build request
	req, err := http.NewRequest("POST", codexRESTBaseURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Measure latency
	startTime := time.Now()
	resp, err := client.Do(req)
	latencyMs := float64(time.Since(startTime).Milliseconds())

	if err != nil {
		return latencyMs, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	// Try to parse response
	var graphqlResp CodexGraphQLResponse
	if err := json.Unmarshal(body, &graphqlResp); err != nil {
		log.Printf("[CODEX-REST][%s] Response parse warning: %v (status: %d)", chainName, err, resp.StatusCode)
	}

	// Check for GraphQL errors
	if len(graphqlResp.Errors) > 0 {
		log.Printf("[CODEX-REST][%s] GraphQL errors: %v", chainName, graphqlResp.Errors[0].Message)
	}

	return latencyMs, resp.StatusCode, nil
}

// monitorCodexREST continuously monitors Codex GraphQL API latency
func monitorCodexREST(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Codex REST API monitor...")
	fmt.Printf("   Monitoring %d chains with 20s interval\n", len(codexRESTChains))
	fmt.Printf("   Endpoint: POST /graphql (GraphQL)\n")
	fmt.Println()

	if config.CodexAPIKey == "" {
		fmt.Println("CODEX_API_KEY not set in .env file. Skipping Codex REST monitor.")
		return
	}

	// Create ticker for 20 second intervals
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	// Run once immediately
	performCodexRESTChecks(config)

	// Then run every 20 seconds
	for {
		select {
		case <-stopChan:
			fmt.Println("Codex REST monitor stopped")
			return
		case <-ticker.C:
			performCodexRESTChecks(config)
		}
	}
}

// performCodexRESTChecks performs GraphQL API calls to all chains
func performCodexRESTChecks(config *Config) {
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")

	for _, chain := range codexRESTChains {
		latencyMs, statusCode, err := callCodexGraphQLAPI(
			config.CodexAPIKey,
			chain.poolAddress,
			chain.networkID,
			chain.chainName,
		)

		if err != nil {
			// Record error
			errorType := "request_error"
			if statusCode >= 500 {
				errorType = "server_error"
			} else if statusCode >= 400 {
				errorType = "client_error"
			} else if statusCode == 0 {
				errorType = "timeout_error"
			}

			RecordRESTError("codex", "graphql", chain.chainName, errorType)

			fmt.Printf("[CODEX-REST][%s][%s] ERROR | Latency: %.0fms | Status: %d | Error: %v\n",
				timestamp,
				chain.chainName,
				latencyMs,
				statusCode,
				err,
			)
			continue
		}

		// Record successful latency measurement
		RecordRESTLatency("codex", "graphql", chain.chainName, latencyMs, statusCode)

		// Log the result
		statusEmoji := "✓"
		if statusCode >= 400 {
			statusEmoji = "✗"
		} else if statusCode >= 300 {
			statusEmoji = "⚠"
		}

		fmt.Printf("[CODEX-REST][%s][%s] %s | Latency: %.0fms | Status: %d\n",
			timestamp,
			chain.chainName,
			statusEmoji,
			latencyMs,
			statusCode,
		)
	}
}

// runCodexRESTMonitor is the entry point for the Codex REST monitor
func runCodexRESTMonitor(config *Config, stopChan <-chan struct{}) {
	monitorCodexREST(config, stopChan)
}
