package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	mobulaWSURL = "wss://api.mobula.io"
)

var mobulaChains = []struct {
	blockchain   string
	blockchainID int64
	chainName    string
	poolAddress  string
}{
	{"solana", 1399811149, "solana", "7qbRF6YsyGuLUVs6Y1q64bdVrfe4ZcUUz1JRdoVNUJnm"},      // SOL/USDC
	{"evm:1", 1, "ethereum", "0x88e6a0c2ddd26feeb64f039a2c41296fcb3f5640"},                // WETH/USDC Uniswap V3
	{"evm:8453", 8453, "base", "0x4c36388be6f416a29c8d8eee81c771ce6be14b18"},              // WETH/USDC Base
	{"evm:56", 56, "bnb", "0x58f876857a02d6762e0101bb5c46a8c1ed44dc16"},                   // WBNB/BUSD PancakeSwap
	{"evm:42161", 42161, "arbitrum", "0xc6962004f452be9203591991d15f6b388e09e8d0"},        // WETH/USDC Arbitrum
}

type MobulaSubscribeMessage struct {
	Type          string        `json:"type"`
	Authorization string        `json:"authorization"`
	Payload       MobulaPayload `json:"payload"`
}

type MobulaPayload struct {
	AssetMode bool         `json:"assetMode"`
	Items     []MobulaItem `json:"items"`
}

type MobulaItem struct {
	Blockchain string `json:"blockchain"`
	Address    string `json:"address"`
}

type MobulaTradeData struct {
	Date           int64   `json:"date"`
	TokenPrice     float64 `json:"tokenPrice"`
	TokenPriceVs   float64 `json:"tokenPriceVs"`
	TokenAmount    float64 `json:"tokenAmount"`
	TokenAmountVs  float64 `json:"tokenAmountVs"`
	TokenAmountUsd float64 `json:"tokenAmountUsd"`
	Type           string  `json:"type"`
	Operation      string  `json:"operation"`
	Blockchain     string  `json:"blockchain"`
	Hash           string  `json:"hash"`
	Sender         string  `json:"sender"`
	Timestamp      int64   `json:"timestamp"`
	Pair           string  `json:"pair"`
}

func connectMobulaWebSocket(apiKey string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(mobulaWSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	return conn, nil
}

func subscribeToMobulaChannel(conn *websocket.Conn, apiKey string) error {
	var items []MobulaItem
	for _, chain := range mobulaChains {
		items = append(items, MobulaItem{
			Blockchain: chain.blockchain,
			Address:    chain.poolAddress,
		})
	}

	subscribeMsg := MobulaSubscribeMessage{
		Type:          "fast-trade",
		Authorization: apiKey,
		Payload: MobulaPayload{
			AssetMode: false,
			Items:     items,
		},
	}

	fmt.Printf("[MOBULA-TRADE] Subscribing to %d pools:\n", len(items))
	for _, item := range items {
		fmt.Printf("   - Blockchain: %s, Address: %s\n", item.Blockchain, item.Address)
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe to fast trades: %w", err)
	}

	return nil
}

func calculateMobulaLag(tradeTimestamp int64, receiveTime time.Time) int64 {
	tradeTime := time.UnixMilli(tradeTimestamp)
	lag := receiveTime.Sub(tradeTime)
	return lag.Milliseconds()
}

func getChainNameForMobula(blockchainName string) string {
	switch blockchainName {
	case "Solana", "solana":
		return "solana"
	case "Base", "base":
		return "base"
	case "BSC", "BNB Smart Chain", "BNB Smart Chain (BEP20)", "bnb":
		return "bnb"
	case "Monad", "monad":
		return "monad"
	default:
		return blockchainName
	}
}

func handleMobulaWebSocketMessages(conn *websocket.Conn, config *Config) {
	messageCount := 0
	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[MOBULA-TRADE] WebSocket read error: %v", err)
			return
		}

		receiveTime := time.Now().UTC()
		messageCount++

		// Try to parse as error response first
		var errorResp map[string]interface{}
		if err := json.Unmarshal(messageBytes, &errorResp); err == nil {
			if errMsg, ok := errorResp["error"]; ok {
				fmt.Printf("[MOBULA-TRADE ERROR] Server returned error: %v\n", errMsg)
				fmt.Printf("[MOBULA-TRADE ERROR] Full response: %v\n", errorResp)
				continue
			}
			if status, ok := errorResp["status"]; ok {
				fmt.Printf("[MOBULA-TRADE STATUS] Server status: %v\n", status)
				if status != "success" && status != "ok" {
					fmt.Printf("[MOBULA-TRADE STATUS] Full response: %v\n", errorResp)
				}
				continue
			}
		}

		var trade MobulaTradeData
		if err := json.Unmarshal(messageBytes, &trade); err != nil {
			continue
		}

		if trade.Hash == "" || trade.Blockchain == "" {
			continue
		}

		lagMs := calculateMobulaLag(trade.Date, receiveTime)

		chainName := getChainNameForMobula(trade.Blockchain)
		timestamp := receiveTime.Format("2006-01-02 15:04:05")
		tradeTime := time.UnixMilli(trade.Date).Format("15:04:05.000")

		txHashShort := trade.Hash
		if len(txHashShort) > 8 {
			txHashShort = txHashShort[:8]
		}

		fmt.Printf("\n[DEBUG] Raw timestamp: %d | Trade time parsed: %s | Receive time: %s | Lag: %dms\n",
			trade.Date, tradeTime, timestamp, lagMs)

		fmt.Printf("[MOBULA-TRADE][%s][%s] New fast trade! Tx: %s... | Type: %s | Volume: $%.2f | Trade time: %s | Lag: %dms\n",
			timestamp,
			chainName,
			txHashShort,
			trade.Type,
			trade.TokenAmountUsd,
			tradeTime,
			lagMs,
		)

		RecordLatency("mobula", chainName, float64(lagMs))
	}
}

func runMobulaMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Mobula Trade WebSocket monitor...")
	fmt.Printf("   Monitoring %d chains with real-time WebSocket\n", len(mobulaChains))
	fmt.Printf("   Measuring TRUE indexation lag (WebSocket push timing)\n")
	fmt.Println()

	if config.MobulaAPIKey == "" {
		fmt.Println("MOBULA_API_KEY not set in .env file. Skipping Mobula monitor.")
		return
	}

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("Mobula Trade monitor stopped")
			return
		default:
			conn, err := connectMobulaWebSocket(config.MobulaAPIKey)
			if err != nil {
				log.Printf("[MOBULA-TRADE] Failed to connect: %v. Retrying in %v...", err, reconnectDelay)
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println("   Connected to Mobula Trade WebSocket")

			if err := subscribeToMobulaChannel(conn, config.MobulaAPIKey); err != nil {
				log.Printf("[MOBULA-TRADE] Failed to subscribe to channel: %v. Retrying in %v...", err, reconnectDelay)
				conn.Close()
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}
			fmt.Println("   Subscribed to fast-trade stream")

			time.Sleep(500 * time.Millisecond)

			fmt.Println("   Configured pools for monitoring:")
			for _, chain := range mobulaChains {
				fmt.Printf("     - %s (%s)\n", chain.chainName, chain.poolAddress)
			}
			fmt.Println()

			// Reset reconnect delay on successful connection
			reconnectDelay = 5 * time.Second

			// This will block until connection error or stopChan
			handleMobulaWebSocketMessages(conn, config)
			conn.Close()

			// Connection died, log and reconnect
			log.Printf("[MOBULA-TRADE] Connection lost. Reconnecting in %v...", reconnectDelay)
			time.Sleep(reconnectDelay)
		}
	}
}
