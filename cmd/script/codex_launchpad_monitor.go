package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================================
// Codex Launchpad Monitor
// Real-time monitoring of new token deployments on launchpads (Pump.fun, etc.)
// Uses onLaunchpadTokenEventBatch subscription
// ============================================================================

const (
	codexLaunchpadWSURL       = "wss://graph.codex.io/graphql"
	codexMaxLaunchpadLagMs    = 120000 // 2 minutes max lag to record
)

// Launchpad networks to monitor
var codexLaunchpadNetworks = []struct {
	NetworkID int
	ChainName string
}{
	{1399811149, "solana"},
	{1, "ethereum"},
	{8453, "base"},
	{56, "bnb"},
	{42161, "arbitrum"},
}

type CodexLaunchpadWSMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type CodexLaunchpadEventPayload struct {
	Data struct {
		OnLaunchpadTokenEventBatch []struct {
			NetworkID     int    `json:"networkId"`
			EventType     string `json:"eventType"`
			LaunchpadName string `json:"launchpadName"`
			Transactions1 *int   `json:"transactions1"`
			Token         struct {
				Address   string `json:"address"`
				Name      string `json:"name"`
				Symbol    string `json:"symbol"`
				CreatedAt int64  `json:"createdAt"`
			} `json:"token"`
		} `json:"onLaunchpadTokenEventBatch"`
	} `json:"data"`
}

func getChainNameFromLaunchpadNetworkID(networkID int) string {
	for _, n := range codexLaunchpadNetworks {
		if n.NetworkID == networkID {
			return n.ChainName
		}
	}
	return fmt.Sprintf("network_%d", networkID)
}

func connectAndMonitorCodexLaunchpad(config *Config, stopChan <-chan struct{}) error {
	dialer := websocket.Dialer{
		Subprotocols: []string{"graphql-transport-ws"},
	}

	conn, _, err := dialer.Dial(codexLaunchpadWSURL, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Connection init
	initMsg := map[string]interface{}{
		"type": "connection_init",
		"payload": map[string]interface{}{
			"Authorization": config.CodexAPIKey,
		},
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	// Wait for ack
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("ack read failed: %w", err)
	}

	var ackMsg CodexLaunchpadWSMessage
	if err := json.Unmarshal(msg, &ackMsg); err != nil || ackMsg.Type != "connection_ack" {
		return fmt.Errorf("unexpected ack: %s", string(msg))
	}

	// Build network filter
	networkIDs := make([]int, len(codexLaunchpadNetworks))
	for i, n := range codexLaunchpadNetworks {
		networkIDs[i] = n.NetworkID
	}

	// Subscribe to onLaunchpadTokenEventBatch
	// Filter for Deployed events only (minimal latency, no metadata wait)
	subMsg := map[string]interface{}{
		"type": "subscribe",
		"id":   "launchpad_monitor",
		"payload": map[string]interface{}{
			"query": `subscription OnLaunchpadEvents($networkFilter: [Int!]) {
				onLaunchpadTokenEventBatch(networkFilter: $networkFilter) {
					networkId
					eventType
					token {
						address
						name
						symbol
						createdAt
					}
					launchpadName
					transactions1
				}
			}`,
			"variables": map[string]interface{}{
				"networkFilter": networkIDs,
			},
		},
	}

	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	fmt.Printf("[CODEX-LAUNCHPAD] Subscribed to %d networks\n", len(networkIDs))

	// Track seen tokens to avoid duplicate metrics
	seenTokens := make(map[string]bool)

	// Read messages - longer timeout since Deployed events can be sparse
	for {
		select {
		case <-stopChan:
			return nil
		default:
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read failed: %w", err)
			}

			receiveTime := time.Now().UTC()

			var wsMsg CodexLaunchpadWSMessage
			if err := json.Unmarshal(message, &wsMsg); err != nil {
				continue
			}

			// Handle ping
			if wsMsg.Type == "ping" {
				pongMsg := map[string]string{"type": "pong"}
				conn.WriteJSON(pongMsg)
				continue
			}

			// Skip non-data messages
			if wsMsg.Type != "next" || wsMsg.Payload == nil {
				continue
			}

			// Parse event payload
			var payload CodexLaunchpadEventPayload
			if err := json.Unmarshal(wsMsg.Payload, &payload); err != nil {
				continue
			}

			// Process events
			for _, event := range payload.Data.OnLaunchpadTokenEventBatch {
				// Only process Deployed or Created events for new token discovery
				if event.EventType != "Deployed" && event.EventType != "Created" {
					continue
				}

				// Skip if already seen
				tokenKey := fmt.Sprintf("%d:%s", event.NetworkID, event.Token.Address)
				if seenTokens[tokenKey] {
					continue
				}
				seenTokens[tokenKey] = true

				// Calculate discovery lag
				if event.Token.CreatedAt == 0 {
					continue
				}

				createdTime := time.Unix(event.Token.CreatedAt, 0)
				discoveryLagMs := receiveTime.Sub(createdTime).Milliseconds()

				// Skip invalid lags
				if discoveryLagMs < 0 || discoveryLagMs > codexMaxLaunchpadLagMs {
					continue
				}

				chainName := getChainNameFromLaunchpadNetworkID(event.NetworkID)

				// Record metric
				RecordPoolDiscoveryLatency("codex-launchpad", chainName, float64(discoveryLagMs))

				// Log the discovery
				timestamp := receiveTime.Format("2006-01-02 15:04:05")
				fmt.Printf("\n[CODEX-LAUNCHPAD][%s][%s] NEW TOKEN %s!\n", timestamp, chainName, event.EventType)
				fmt.Printf("   Token: %s (%s)\n", event.Token.Symbol, event.Token.Name)
				fmt.Printf("   Address: %s\n", event.Token.Address)
				fmt.Printf("   Launchpad: %s\n", event.LaunchpadName)
				fmt.Printf("   Discovery lag: %dms (%.2fs)\n", discoveryLagMs, float64(discoveryLagMs)/1000.0)
				fmt.Println()

				// Queue token for metadata coverage check
				QueueTokenForMetadataCheck(TokenToCheck{
					Address:    event.Token.Address,
					ChainID:    fmt.Sprintf("evm:%d", event.NetworkID),
					Symbol:     event.Token.Symbol,
					Name:       event.Token.Name,
					DetectedAt: receiveTime,
				})
			}

			// Cleanup old seen tokens periodically (keep last 10000)
			if len(seenTokens) > 10000 {
				seenTokens = make(map[string]bool)
			}
		}
	}
}

func runCodexLaunchpadMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Codex Launchpad monitor...")
	fmt.Printf("   Monitoring launchpads: Pump.fun, Four.meme, Meteora, Zora, etc.\n")
	fmt.Printf("   Event types: Deployed, Created\n")
	fmt.Println()

	if config.CodexAPIKey == "" {
		fmt.Println("CODEX_API_KEY not set in .env file. Skipping Codex Launchpad monitor.")
		return
	}

	fmt.Println("   Monitoring chains:")
	for _, n := range codexLaunchpadNetworks {
		fmt.Printf("     - %s (networkId: %d)\n", n.ChainName, n.NetworkID)
	}
	fmt.Println()
	fmt.Println("   Waiting for new token deployments...")
	fmt.Println()

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("Codex Launchpad monitor stopped")
			return
		default:
			err := connectAndMonitorCodexLaunchpad(config, stopChan)
			if err != nil {
				log.Printf("[CODEX-LAUNCHPAD] Connection error: %v. Reconnecting in %v...", err, reconnectDelay)
				RecordPoolDiscoveryError("codex-launchpad", "connection_error")

				select {
				case <-stopChan:
					return
				case <-time.After(reconnectDelay):
					reconnectDelay = reconnectDelay * 2
					if reconnectDelay > maxReconnectDelay {
						reconnectDelay = maxReconnectDelay
					}
				}
			} else {
				reconnectDelay = 5 * time.Second
			}
		}
	}
}
