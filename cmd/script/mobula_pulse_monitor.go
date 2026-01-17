package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	mobulaPulseWSURL = "wss://pulse-v2-api.mobula.io"
)

// Chains to monitor for new pools - focus on chains with active launchpads
var pulseChains = []string{
	"solana:solana", // Solana (Pump.fun, Meteora, BAGS)
	"evm:56",        // BNB (Four.meme, Flap)
}

type PulseSubscribeMessage struct {
	Type          string       `json:"type"`
	Authorization string       `json:"authorization"`
	Payload       PulsePayload `json:"payload"`
}

type PulsePayload struct {
	Model      string      `json:"model"`
	AssetMode  bool        `json:"assetMode"`
	ChainID    []string    `json:"chainId"`
	PoolTypes  []string    `json:"poolTypes,omitempty"`
	Compressed bool        `json:"compressed"`
	Views      []PulseView `json:"views,omitempty"`
}

type PulseView struct {
	Name      string                 `json:"name"`
	Filters   map[string]interface{} `json:"filters,omitempty"`
	SortBy    string                 `json:"sortBy,omitempty"`
	SortOrder string                 `json:"sortOrder,omitempty"`
	Limit     int                    `json:"limit,omitempty"`
}

type PulseV2NewTokenMessage struct {
	Type    string              `json:"type"`
	Payload PulseV2TokenPayload `json:"payload"`
}

type PulseV2TokenPayload struct {
	ViewName  string            `json:"viewName"`
	Token     PulseV2TokenOuter `json:"token"`
	CreatedAt int64             `json:"created_at"` // Timestamp in milliseconds
	Source    string            `json:"source"`
}

type PulseV2TokenOuter struct {
	Token  PulseV2Token `json:"token"`
	Source string       `json:"source"` // Source is at this level, not inside token
}

type PulseV2Token struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Symbol    string `json:"symbol"`
	ChainID   string `json:"chainId"`
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"` // ISO 8601 timestamp
}

func connectMobulaPulseWebSocket(apiKey string) (*websocket.Conn, error) {
	// Add API key to request headers
	headers := make(map[string][]string)
	headers["Authorization"] = []string{apiKey}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(mobulaPulseWSURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Pulse WebSocket: %w", err)
	}

	return conn, nil
}

func subscribeToPulse(conn *websocket.Conn, apiKey string) error {
	subscribeMsg := PulseSubscribeMessage{
		Type:          "pulse-v2",
		Authorization: apiKey,
		Payload: PulsePayload{
			Model:      "default",
			AssetMode:  true,
			ChainID:    pulseChains,
			Compressed: false,
			Views: []PulseView{
				{
					Name:      "new",
					SortBy:    "created_at",
					SortOrder: "desc",
					Limit:     50,
				},
			},
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe to Pulse: %w", err)
	}

	return nil
}

// Launchpad sources to filter - must match Codex launchpads for fair comparison
// Source names from Mobula Pulse V2 API
var launchpadSources = map[string]bool{
	"pumpfun":      true, // Pump.fun (Solana)
	"pump.fun":     true,
	"meteora":      true, // Meteora (Solana)
	"meteora-dbc":  true, // Meteora DBC (Solana)
	"meteoradbc":   true,
	"fourmeme":     true, // Four.meme (BNB)
	"four.meme":    true,
	"flap":         true, // Flap (BNB) - similar to Four.meme
	"zora":         true, // Zora (Base)
	"baseapp":      true, // Baseapp (Base)
	"bags":         true, // BAGS (Solana)
	"moonshot":     true, // Moonshot
	"raydium_cpmm": true, // Raydium launchpad pools
}

func isLaunchpadSource(source string) bool {
	return launchpadSources[source]
}

func getChainNameForPulse(chainID string) string {
	switch chainID {
	case "solana:solana":
		return "solana"
	case "evm:56":
		return "bnb"
	case "evm:8453":
		return "base"
	case "evm:143":
		return "monad"
	default:
		return chainID
	}
}

func handlePulseV2Messages(conn *websocket.Conn, config *Config) {
	messageCount := 0
	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[MOBULA-PULSE] WebSocket read error: %v", err)
			return
		}

		receiveTime := time.Now().UTC()
		messageCount++

		// Try to parse as generic message first to get the type
		var genericMsg map[string]interface{}
		if err := json.Unmarshal(messageBytes, &genericMsg); err != nil {
			fmt.Printf("[MOBULA-PULSE DEBUG] Failed to parse message: %s\n", string(messageBytes[:100]))
			continue
		}

		msgType, ok := genericMsg["type"].(string)
		if !ok {
			continue
		}

		// Handle different message types
		switch msgType {
		case "new-token":
			var tokenMsg PulseV2NewTokenMessage
			if err := json.Unmarshal(messageBytes, &tokenMsg); err != nil {
				log.Printf("[MOBULA-PULSE] Failed to parse new-token message: %v", err)
				continue
			}

			token := tokenMsg.Payload.Token.Token
			// Source is at payload.token level, not payload.token.token level
			source := tokenMsg.Payload.Token.Source
			if source == "" {
				source = tokenMsg.Payload.Source
			}
			if source == "" {
				source = token.Source
			}

			// Filter: only process launchpad sources for fair comparison with Codex
			if !isLaunchpadSource(source) {
				// Skip non-launchpad tokens (DEX pools like Uniswap, Raydium, etc.)
				continue
			}

			// Parse the created_at timestamp (ISO 8601 format)
			var createdAt time.Time
			var err error

			if token.CreatedAt != "" {
				createdAt, err = time.Parse(time.RFC3339, token.CreatedAt)
			}

			if err != nil || createdAt.IsZero() {
				continue
			}

			// Calculate discovery latency: time from pool creation to our discovery
			discoveryLagMs := receiveTime.Sub(createdAt).Milliseconds()

			// Determine chain name from chainId
			chainName := getChainNameForPulse(token.ChainID)
			if chainName == token.ChainID {
				// If not found in our mapping, use it as-is
				chainName = token.ChainID
			}

			timestamp := receiveTime.Format("2006-01-02 15:04:05")
			createdAtFormatted := createdAt.Format("15:04:05.000")

			fmt.Printf("\n[MOBULA-PULSE][%s][%s] LAUNCHPAD TOKEN DETECTED!\n", timestamp, chainName)
			fmt.Printf("   Token: %s (%s)\n", token.Symbol, token.Name)
			fmt.Printf("   Address: %s\n", token.Address)
			fmt.Printf("   Created on-chain: %s\n", createdAtFormatted)
			fmt.Printf("   Discovery lag: %dms\n", discoveryLagMs)
			fmt.Printf("   Launchpad: %s\n\n", source)

			// Record pool discovery latency metric
			RecordPoolDiscoveryLatency("mobula-pulse", chainName, float64(discoveryLagMs))

			// Queue token for metadata coverage check
			QueueTokenForMetadataCheck(TokenToCheck{
				Address:    token.Address,
				ChainID:    token.ChainID,
				Symbol:     token.Symbol,
				Name:       token.Name,
				DetectedAt: receiveTime,
			})

		case "update-token":
			// Silent - just continue
			continue

		case "ping", "pong":
			// Ignore ping/pong messages
			continue

		case "error":
			fmt.Printf("[MOBULA-PULSE ERROR] Received error: %v\n", genericMsg)

		default:
			continue
		}
	}
}

func runMobulaPulseMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Mobula Pulse V2 monitor...")
	fmt.Printf("   Monitoring %d chains for LAUNCHPAD TOKENS ONLY\n", len(pulseChains))
	fmt.Printf("   Launchpads: Pump.fun, Meteora, Four.meme, Zora, Baseapp, BAGS, Moonshot\n")
	fmt.Printf("   Measuring discovery latency (on-chain creation → Mobula indexation)\n")
	fmt.Println()

	if config.MobulaAPIKey == "" {
		fmt.Println("MOBULA_API_KEY not set in .env file. Skipping Mobula Pulse monitor.")
		return
	}

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("Mobula Pulse monitor stopped")
			return
		default:
			conn, err := connectMobulaPulseWebSocket(config.MobulaAPIKey)
			if err != nil {
				log.Printf("[MOBULA-PULSE] Failed to connect: %v. Retrying in %v...", err, reconnectDelay)
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println("   Connected to Mobula Pulse WebSocket")

			if err := subscribeToPulse(conn, config.MobulaAPIKey); err != nil {
				log.Printf("[MOBULA-PULSE] Failed to subscribe: %v. Retrying in %v...", err, reconnectDelay)
				conn.Close()
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}
			fmt.Println("   Subscribed to new token/pool creation stream")

			fmt.Println("   Monitoring chains:")
			for _, chain := range pulseChains {
				fmt.Printf("     - %s\n", getChainNameForPulse(chain))
			}
			fmt.Println()
			fmt.Println("   Waiting for new pools to be created...")
			fmt.Println()

			// Reset reconnect delay on successful connection
			reconnectDelay = 5 * time.Second

			// This will block until connection error or stopChan
			handlePulseV2Messages(conn, config)
			conn.Close()

			// Connection died, log and reconnect
			log.Printf("[MOBULA-PULSE] Connection lost. Reconnecting in %v...", reconnectDelay)
			time.Sleep(reconnectDelay)
		}
	}
}
