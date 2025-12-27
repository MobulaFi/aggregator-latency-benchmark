package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	codexWSURL = "wss://graph.codex.io/graphql"
)

var codexChains = []struct {
	networkID   int
	chainName   string
	poolAddress string
}{
	{1399811149, "solana", "7qbRF6YsyGuLUVs6Y1q64bdVrfe4ZcUUz1JRdoVNUJnm"}, // SOL/USDC Raydium
	{1, "ethereum", "0x88e6a0c2ddd26feeb64f039a2c41296fcb3f5640"},           // WETH/USDC Uniswap V3
	{8453, "base", "0x4c36388be6f416a29c8d8eee81c771ce6be14b18"},            // WETH/USDC Base
	{56, "bnb", "0x58f876857a02d6762e0101bb5c46a8c1ed44dc16"},               // WBNB/BUSD PancakeSwap
	{42161, "arbitrum", "0xc6962004f452be9203591991d15f6b388e09e8d0"},       // WETH/USDC Uniswap V3 Arbitrum
}

type CodexWSMessage struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

type CodexConnectionInit struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

type CodexSubscribe struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id"`
	Payload map[string]interface{} `json:"payload"`
}

type CodexEvent struct {
	NetworkID          int    `json:"networkId"`
	BlockNumber        int64  `json:"blockNumber"`
	Timestamp          int64  `json:"timestamp"`
	TransactionHash    string `json:"transactionHash"`
	EventType          string `json:"eventType"`
	Token0Address      string `json:"token0Address"`
	Token1Address      string `json:"token1Address"`
	Token0SwapValueUsd string `json:"token0SwapValueUsd"`
	Token1SwapValueUsd string `json:"token1SwapValueUsd"`
}

// For onUnconfirmedEventsCreated (Solana only)
type UnconfirmedCodexEvent struct {
	Address          string `json:"address"`
	BlockHash        string `json:"blockHash"`
	BlockNumber      int64  `json:"blockNumber"`
	EventType        string `json:"eventType"`
	Maker            string `json:"maker"`
	NetworkID        int    `json:"networkId"`
	Timestamp        int64  `json:"timestamp"`
	TransactionHash  string `json:"transactionHash"`
}

type CodexEventData struct {
	Data struct {
		OnEventsCreated struct {
			Address   string       `json:"address"`
			NetworkID int          `json:"networkId"`
			Events    []CodexEvent `json:"events"`
		} `json:"onEventsCreated"`
		OnUnconfirmedEventsCreated struct {
			Address   string                  `json:"address"`
			NetworkID int                     `json:"networkId"`
			Events    []UnconfirmedCodexEvent `json:"events"`
		} `json:"onUnconfirmedEventsCreated"`
	} `json:"data"`
}

func connectCodexWebSocket(apiKey string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		Subprotocols: []string{"graphql-transport-ws"},
	}

	conn, _, err := dialer.Dial(codexWSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	initMsg := CodexConnectionInit{
		Type: "connection_init",
		Payload: map[string]interface{}{
			"Authorization": apiKey,
		},
	}

	if err := conn.WriteJSON(initMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send connection_init: %w", err)
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read connection_ack: %w", err)
	}

	var ackMsg CodexWSMessage
	if err := json.Unmarshal(msg, &ackMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to parse connection_ack: %w", err)
	}

	if ackMsg.Type != "connection_ack" {
		conn.Close()
		return nil, fmt.Errorf("expected connection_ack, got: %s", ackMsg.Type)
	}

	fmt.Println("Connection acknowledged by Codex")

	return conn, nil
}

func subscribeToCodexPool(conn *websocket.Conn, poolAddress string, networkID int, subID string, chainName string) error {
	var query string
	var variables map[string]interface{}

	// Solana uses onUnconfirmedEventsCreated (lowest latency)
	if networkID == 1399811149 { // Solana
		query = `subscription OnUnconfirmedPoolEvents($id: String!) {
			onUnconfirmedEventsCreated(id: $id) {
				address
				networkId
				events {
					address
					blockHash
					blockNumber
					eventType
					maker
					networkId
					timestamp
					transactionHash
				}
			}
		}`

		pairID := fmt.Sprintf("%s:%d", poolAddress, networkID)
		variables = map[string]interface{}{
			"id": pairID,
		}
	} else { // BNB, Base, Monad
		query = `subscription OnPoolEvents($address: String!, $networkId: Int!) {
			onEventsCreated(address: $address, networkId: $networkId) {
				address
				networkId
				events {
					networkId
					blockNumber
					timestamp
					transactionHash
					eventType
					token0Address
					token1Address
					token0SwapValueUsd
					token1SwapValueUsd
				}
			}
		}`

		variables = map[string]interface{}{
			"address":   poolAddress,
			"networkId": networkID,
		}
	}

	subscribeMsg := CodexSubscribe{
		Type: "subscribe",
		ID:   subID,
		Payload: map[string]interface{}{
			"query":     query,
			"variables": variables,
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

func calculateCodexLag(blockTimestamp int64, receiveTime time.Time) int64 {
	tradeTime := time.Unix(blockTimestamp, 0)
	lag := receiveTime.Sub(tradeTime)
	return lag.Milliseconds()
}

func getChainNameForCodex(networkID int) string {
	for _, chain := range codexChains {
		if chain.networkID == networkID {
			return chain.chainName
		}
	}
	return fmt.Sprintf("network_%d", networkID)
}

func handleCodexWebSocketMessages(conn *websocket.Conn, config *Config) {
	messageCount := 0
	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[CODEX] WebSocket read error: %v", err)
			return
		}

		receiveTime := time.Now().UTC()
		messageCount++

		var genericMsg CodexWSMessage
		if err := json.Unmarshal(messageBytes, &genericMsg); err != nil {
			continue
		}

		switch genericMsg.Type {
		case "next":
			if genericMsg.Payload == nil {
				continue
			}

			payloadBytes, _ := json.Marshal(genericMsg.Payload)
			var eventData CodexEventData
			if err := json.Unmarshal(payloadBytes, &eventData); err != nil {
				continue
			}

			// Try unconfirmed events first (Solana)
			if len(eventData.Data.OnUnconfirmedEventsCreated.Events) > 0 {
				for _, event := range eventData.Data.OnUnconfirmedEventsCreated.Events {
					if event.EventType != "Swap" {
						continue
					}

					if event.TransactionHash == "" {
						continue
					}

					lagMs := calculateCodexLag(event.Timestamp, receiveTime)

					chainName := getChainNameForCodex(event.NetworkID)
					timestamp := receiveTime.Format("2006-01-02 15:04:05")

					txHashShort := event.TransactionHash
					if len(txHashShort) > 8 {
						txHashShort = txHashShort[:8]
					}

					fmt.Printf("[CODEX][%s][%s][UNCONFIRMED] Tx: %s... | Block: %d | Lag: %dms\n",
						timestamp,
						chainName,
						txHashShort,
						event.BlockNumber,
						lagMs,
					)

					RecordLatency("codex", chainName, float64(lagMs))
				}
			}

			// Try confirmed events (BNB, Base, Monad)
			if len(eventData.Data.OnEventsCreated.Events) > 0 {
				for _, event := range eventData.Data.OnEventsCreated.Events {
					if event.EventType != "Swap" {
						continue
					}

					if event.TransactionHash == "" {
						continue
					}

					lagMs := calculateCodexLag(event.Timestamp, receiveTime)

					chainName := getChainNameForCodex(event.NetworkID)
					timestamp := receiveTime.Format("2006-01-02 15:04:05")

					txHashShort := event.TransactionHash
					if len(txHashShort) > 8 {
						txHashShort = txHashShort[:8]
					}

					fmt.Printf("[CODEX][%s][%s][CONFIRMED] Tx: %s... | Block: %d | Lag: %dms\n",
						timestamp,
						chainName,
						txHashShort,
						event.BlockNumber,
						lagMs,
					)

					RecordLatency("codex", chainName, float64(lagMs))
				}
			}

		case "error":
			fmt.Printf("[CODEX ERROR] Received error: %+v\n", genericMsg.Payload)

		case "complete":
			fmt.Printf("[CODEX] Subscription %s completed\n", genericMsg.ID)

		case "ka":
			continue

		default:
			continue
		}
	}
}

func runCodexMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Codex WebSocket monitor...")
	fmt.Printf("Monitoring %d chains with real-time GraphQL WebSocket\n", len(codexChains))
	fmt.Printf("Solana: onUnconfirmedEventsCreated (faster)\n")
	fmt.Printf("Others: onEventsCreated\n")
	fmt.Println()

	if config.CodexAPIKey == "" {
		fmt.Println("CODEX_API_KEY not set in .env file. Skipping Codex monitor.")
		return
	}

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("Codex monitor stopped")
			return
		default:
			conn, err := connectCodexWebSocket(config.CodexAPIKey)
			if err != nil {
				log.Printf("[CODEX] Failed to connect: %v. Retrying in %v...", err, reconnectDelay)
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println("Connected to Codex WebSocket")

			// Subscribe to all chains
			allSubscribed := true
			for i, chain := range codexChains {
				subID := fmt.Sprintf("sub_%d", i+1)
				if err := subscribeToCodexPool(conn, chain.poolAddress, chain.networkID, subID, chain.chainName); err != nil {
					log.Printf("[CODEX] Failed to subscribe to %s pool: %v. Will reconnect...", chain.chainName, err)
					allSubscribed = false
					break
				}

				subscriptionType := "confirmed"
				if chain.networkID == 1399811149 { // Solana
					subscriptionType = "unconfirmed"
				}
				fmt.Printf("Subscribed to %s pool (%s) [%s]\n", chain.chainName, chain.poolAddress, subscriptionType)
				time.Sleep(200 * time.Millisecond)
			}

			if !allSubscribed {
				conn.Close()
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println()

			// Reset reconnect delay on successful connection and subscription
			reconnectDelay = 5 * time.Second

			// This will block until connection error or stopChan
			handleCodexWebSocketMessages(conn, config)
			conn.Close()

			// Connection died, log and reconnect
			log.Printf("[CODEX] Connection lost. Reconnecting in %v...", reconnectDelay)
			time.Sleep(reconnectDelay)
		}
	}
}
