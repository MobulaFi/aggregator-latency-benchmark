package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================================
// Head Lag Monitor
// Measures indexation latency: time between on-chain event and WebSocket receipt
// ============================================================================

// Pool configurations for head lag monitoring
type HeadLagPool struct {
	Name       string // Human readable name
	Blockchain string // For Mobula: "evm:1", "solana", etc.
	NetworkID  int    // For Codex: 1, 1399811149, etc.
	Address    string // Pool address
	ChainName  string // Normalized chain name for metrics
}

// Pools to monitor - high activity pools for accurate lag measurement
var headLagPools = []HeadLagPool{
	{
		Name:       "ETH/USDC Uniswap V3",
		Blockchain: "evm:1",
		NetworkID:  1,
		Address:    "0x88e6a0c2ddd26feeb64f039a2c41296fcb3f5640",
		ChainName:  "ethereum",
	},
	{
		Name:       "SOL/USDC Raydium",
		Blockchain: "solana",
		NetworkID:  1399811149,
		Address:    "7qbRF6YsyGuLUVs6Y1q64bdVrfe4ZcUUz1JRdoVNUJnm",
		ChainName:  "solana",
	},
	{
		Name:       "WETH/USDC Base",
		Blockchain: "evm:8453",
		NetworkID:  8453,
		Address:    "0x4c36388be6f416a29c8d8eee81c771ce6be14b18",
		ChainName:  "base",
	},
	{
		Name:       "WBNB/BUSD PancakeSwap",
		Blockchain: "evm:56",
		NetworkID:  56,
		Address:    "0x58f876857a02d6762e0101bb5c46a8c1ed44dc16",
		ChainName:  "bnb",
	},
	{
		Name:       "WETH/USDC Arbitrum",
		Blockchain: "evm:42161",
		NetworkID:  42161,
		Address:    "0xc6962004f452be9203591991d15f6b388e09e8d0",
		ChainName:  "arbitrum",
	},
}

// ============================================================================
// Mobula WebSocket Monitor
// ============================================================================

type MobulaTradeEvent struct {
	Blockchain string  `json:"blockchain"`
	Date       int64   `json:"date"`      // On-chain timestamp (ms)
	Timestamp  int64   `json:"timestamp"` // When Mobula processed it (ms)
	Hash       string  `json:"hash"`
	Pair       string  `json:"pair"`
	Type       string  `json:"type"`
	TokenPrice float64 `json:"tokenPrice"`
}

func runMobulaHeadLagMonitor(config *Config, stopChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	if config.MobulaAPIKey == "" {
		fmt.Println("[HEAD-LAG][MOBULA] API key not set, skipping")
		return
	}

	fmt.Println("[HEAD-LAG][MOBULA] Starting WebSocket monitor...")

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("[HEAD-LAG][MOBULA] Monitor stopped")
			return
		default:
			err := connectAndMonitorMobula(config, stopChan)
			if err != nil {
				log.Printf("[HEAD-LAG][MOBULA] Connection error: %v. Reconnecting in %v...", err, reconnectDelay)
				
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
				// Reset delay on clean disconnect
				reconnectDelay = 5 * time.Second
			}
		}
	}
}

func connectAndMonitorMobula(config *Config, stopChan <-chan struct{}) error {
	conn, _, err := websocket.DefaultDialer.Dial("wss://api.mobula.io", nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Build subscription items
	var items []map[string]interface{}
	for _, pool := range headLagPools {
		items = append(items, map[string]interface{}{
			"blockchain": pool.Blockchain,
			"address":    pool.Address,
		})
	}

	// Subscribe to fast-trade
	subscribeMsg := map[string]interface{}{
		"type":          "fast-trade",
		"authorization": config.MobulaAPIKey,
		"payload": map[string]interface{}{
			"assetMode": false,
			"items":     items,
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	fmt.Printf("[HEAD-LAG][MOBULA] Subscribed to %d pools\n", len(items))

	// Start ping goroutine
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				if err := conn.WriteJSON(map[string]string{"event": "ping"}); err != nil {
					return
				}
			}
		}
	}()
	defer close(pingDone)

	// Read messages
	for {
		select {
		case <-stopChan:
			return nil
		default:
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read failed: %w", err)
			}

			// Parse message
			var trade MobulaTradeEvent
			if err := json.Unmarshal(message, &trade); err != nil {
				continue
			}

			// Skip non-trade messages (pong, etc)
			if trade.Hash == "" || trade.Date == 0 {
				continue
			}

			// Calculate head lag
			receiveTime := time.Now().UTC()
			onChainTime := time.UnixMilli(trade.Date)
			lagMs := receiveTime.Sub(onChainTime).Milliseconds()
			lagSeconds := float64(lagMs) / 1000.0

			// Get chain name from pool config
			chainName := getChainNameFromBlockchain(trade.Blockchain)

			// Record metric
			RecordHeadLag("mobula", chainName, lagMs, lagSeconds, config.MonitorRegion)

			// Log occasionally (not every trade)
			if lagMs > 5000 || time.Now().Second()%30 == 0 {
				timestamp := receiveTime.Format("15:04:05")
				fmt.Printf("[HEAD-LAG][MOBULA][%s][%s] Lag: %.2fs | Tx: %s\n",
					timestamp, chainName, lagSeconds, trade.Hash)
			}
		}
	}
}

func getChainNameFromBlockchain(blockchain string) string {
	switch blockchain {
	case "Ethereum", "evm:1":
		return "ethereum"
	case "Solana", "solana":
		return "solana"
	case "Base", "evm:8453":
		return "base"
	case "BNB Smart Chain (BEP20)", "BSC", "evm:56":
		return "bnb"
	case "Arbitrum", "evm:42161":
		return "arbitrum"
	default:
		return blockchain
	}
}

// ============================================================================
// Codex WebSocket Monitor (using Defined.fi session auth)
// ============================================================================

type CodexWSMessage struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type CodexEventData struct {
	Data struct {
		OnEventsCreated struct {
			Address   string `json:"address"`
			NetworkID int    `json:"networkId"`
			Events    []struct {
				BlockNumber     int64  `json:"blockNumber"`
				Timestamp       int64  `json:"timestamp"`
				TransactionHash string `json:"transactionHash"`
				EventType       string `json:"eventType"`
			} `json:"events"`
		} `json:"onEventsCreated"`
	} `json:"data"`
}

func runCodexHeadLagMonitor(config *Config, stopChan <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Println("[HEAD-LAG][CODEX] Starting WebSocket monitor (via Defined.fi auth)...")

	reconnectDelay := 30 * time.Second
	maxReconnectDelay := 5 * time.Minute

	for {
		select {
		case <-stopChan:
			fmt.Println("[HEAD-LAG][CODEX] Monitor stopped")
			return
		default:
			err := connectAndMonitorCodex(config, stopChan)
			if err != nil {
				log.Printf("[HEAD-LAG][CODEX] Connection error: %v", err)

				// Check if it's a rate limit error
				if strings.Contains(err.Error(), "rate limited (429)") {
					log.Printf("[HEAD-LAG][CODEX] ⚠ Rate limited - waiting %v before retry", reconnectDelay)
					// Longer delay for rate limits
					reconnectDelay = 2 * time.Minute
				} else if strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "401") {
					log.Printf("[HEAD-LAG][CODEX] Authentication error - invalidating token cache")
					InvalidateTokenCache()
				}

				log.Printf("[HEAD-LAG][CODEX] Reconnecting in %v...", reconnectDelay)
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

func connectAndMonitorCodex(config *Config, stopChan <-chan struct{}) error {
	// Get JWT token from Defined.fi session cookie (required - cookie alone doesn't work)
	jwtToken, err := GetDefinedJWTToken(config.DefinedSessionCookie)
	if err != nil {
		return fmt.Errorf("failed to get JWT token: %w", err)
	}

	dialer := websocket.Dialer{
		Subprotocols: []string{"graphql-transport-ws"},
	}

	conn, _, err := dialer.Dial("wss://graph.codex.io/graphql", nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Connection init with Bearer token
	initMsg := map[string]interface{}{
		"type": "connection_init",
		"payload": map[string]interface{}{
			"Authorization": fmt.Sprintf("Bearer %s", jwtToken),
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

	var ackMsg CodexWSMessage
	if err := json.Unmarshal(msg, &ackMsg); err != nil || ackMsg.Type != "connection_ack" {
		return fmt.Errorf("unexpected ack: %s", string(msg))
	}

	// Subscribe to each pool
	for i, pool := range headLagPools {
		subID := fmt.Sprintf("headlag_%d", i)

		subMsg := map[string]interface{}{
			"type": "subscribe",
			"id":   subID,
			"payload": map[string]interface{}{
				"query": `subscription OnPoolEvents($address: String!, $networkId: Int!) {
					onEventsCreated(address: $address, networkId: $networkId) {
						address
						networkId
						events {
							blockNumber
							timestamp
							transactionHash
							eventType
						}
					}
				}`,
				"variables": map[string]interface{}{
					"address":   pool.Address,
					"networkId": pool.NetworkID,
				},
			},
		}

		if err := conn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("subscribe to %s failed: %w", pool.Name, err)
		}

		time.Sleep(100 * time.Millisecond) // Small delay between subscriptions
	}

	fmt.Printf("[HEAD-LAG][CODEX] Subscribed to %d pools\n", len(headLagPools))

	// Read messages
	for {
		select {
		case <-stopChan:
			return nil
		default:
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				return fmt.Errorf("read failed: %w", err)
			}

			// Parse message
			var wsMsg CodexWSMessage
			if err := json.Unmarshal(message, &wsMsg); err != nil {
				continue
			}

			// Skip non-data messages
			if wsMsg.Type != "next" || wsMsg.Payload == nil {
				continue
			}

			// Parse event data
			payloadBytes, _ := json.Marshal(wsMsg.Payload)
			var eventData CodexEventData
			if err := json.Unmarshal(payloadBytes, &eventData); err != nil {
				continue
			}

			events := eventData.Data.OnEventsCreated.Events
			if len(events) == 0 {
				continue
			}

			networkID := eventData.Data.OnEventsCreated.NetworkID

			for _, event := range events {
				if event.EventType != "Swap" || event.TransactionHash == "" {
					continue
				}

				// Calculate head lag
				receiveTime := time.Now().UTC()
				onChainTime := time.Unix(event.Timestamp, 0)
				lagMs := receiveTime.Sub(onChainTime).Milliseconds()
				lagSeconds := float64(lagMs) / 1000.0

				// Get chain name
				chainName := getChainNameFromNetworkID(networkID)

				// Record metrics
				RecordHeadLag("codex", chainName, lagMs, lagSeconds, config.MonitorRegion)
				RecordCodexBlockNumber(chainName, event.BlockNumber, config.MonitorRegion)

				// Log occasionally
				if lagMs > 5000 || time.Now().Second()%30 == 0 {
					timestamp := receiveTime.Format("15:04:05")
					fmt.Printf("[HEAD-LAG][CODEX][%s][%s] Lag: %.2fs | Block: %d | Tx: %s\n",
						timestamp, chainName, lagSeconds, event.BlockNumber, event.TransactionHash)
				}
			}
		}
	}
}

func getChainNameFromNetworkID(networkID int) string {
	switch networkID {
	case 1:
		return "ethereum"
	case 1399811149:
		return "solana"
	case 8453:
		return "base"
	case 56:
		return "bnb"
	case 42161:
		return "arbitrum"
	default:
		return fmt.Sprintf("network_%d", networkID)
	}
}

// ============================================================================
// Main Head Lag Monitor
// ============================================================================

func runHeadLagMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              HEAD LAG MONITOR (WebSocket-based)              ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Measures: Time between on-chain event and WebSocket receipt ║")
	fmt.Println("║  Providers: Mobula + Codex + GeckoTerminal                   ║")
	fmt.Printf("║  Pools: %d high-activity pools across 5 chains               ║\n", len(headLagPools))
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	var wg sync.WaitGroup

	// Start Mobula monitor
	wg.Add(1)
	go runMobulaHeadLagMonitor(config, stopChan, &wg)

	// Start Codex monitor
	wg.Add(1)
	go runCodexHeadLagMonitor(config, stopChan, &wg)

	// Start GeckoTerminal monitor
	wg.Add(1)
	go runGeckoTerminalHeadLagMonitor(config, stopChan, &wg)

	// Wait for all to finish
	wg.Wait()
	fmt.Println("[HEAD-LAG] All monitors stopped")
}
