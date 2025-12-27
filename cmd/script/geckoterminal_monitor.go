package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	coinGeckoWSURL = "wss://stream.coingecko.com/v1"
)

var coinGeckoChains = []struct {
	networkID   string
	chainName   string
	poolAddress string
}{
	{"solana", "solana", "7qbRF6YsyGuLUVs6Y1q64bdVrfe4ZcUUz1JRdoVNUJnm"},             // SOL/USDC
	{"eth", "ethereum", "0x88e6a0c2ddd26feeb64f039a2c41296fcb3f5640"},                // WETH/USDC Uniswap V3
	{"base", "base", "0x4c36388be6f416a29c8d8eee81c771ce6be14b18"},                   // WETH/USDC Base
	{"bsc", "bnb", "0x58f876857a02d6762e0101bb5c46a8c1ed44dc16"},                     // WBNB/BUSD PancakeSwap
	{"arbitrum", "arbitrum", "0xc6962004f452be9203591991d15f6b388e09e8d0"},           // WETH/USDC Arbitrum
}

type WSCommand struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier,omitempty"`
	Data       string `json:"data,omitempty"`
}

type CoinGeckoMessage struct {
	Type       string          `json:"type,omitempty"`
	Message    json.RawMessage `json:"message,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
}

type TradeData struct {
	C  string  `json:"c"`
	N  string  `json:"n"`
	Pa string  `json:"pa"`
	Tx string  `json:"tx"`
	Ty string  `json:"ty"`
	To float64 `json:"to"`
	Vo float64 `json:"vo"`
	Pc float64 `json:"pc"`
	Pu float64 `json:"pu"`
	T  int64   `json:"t"`
}

func connectCoinGeckoWebSocket(apiKey string) (*websocket.Conn, error) {
	url := fmt.Sprintf("%s?x_cg_pro_api_key=%s", coinGeckoWSURL, apiKey)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	return conn, nil
}

func subscribeToCoinGeckoChannel(conn *websocket.Conn) error {
	subscribeCmd := WSCommand{
		Command:    "subscribe",
		Identifier: `{"channel":"OnchainTrade"}`,
	}

	if err := conn.WriteJSON(subscribeCmd); err != nil {
		return fmt.Errorf("failed to subscribe to channel: %w", err)
	}

	return nil
}

func setPoolsForCoinGecko(conn *websocket.Conn, pools []string) error {
	poolsJSON, err := json.Marshal(pools)
	if err != nil {
		return fmt.Errorf("failed to marshal pools: %w", err)
	}

	dataPayload := fmt.Sprintf(`{"network_id:pool_addresses":%s,"action":"set_pools"}`, string(poolsJSON))

	messageCmd := WSCommand{
		Command:    "message",
		Identifier: `{"channel":"OnchainTrade"}`,
		Data:       dataPayload,
	}

	if err := conn.WriteJSON(messageCmd); err != nil {
		return fmt.Errorf("failed to set pools: %w", err)
	}

	return nil
}

func calculateCoinGeckoLag(tradeTimestamp int64, receiveTime time.Time) int64 {
	tradeTime := time.UnixMilli(tradeTimestamp)
	lag := receiveTime.Sub(tradeTime)
	return lag.Milliseconds()
}

func getChainNameForCoinGecko(networkID string) string {
	for _, chain := range coinGeckoChains {
		if chain.networkID == networkID {
			return chain.chainName
		}
	}
	// Handle monad variations
	if networkID == "monad-testnet" || networkID == "monad-mainnet" || networkID == "monad" {
		return "monad"
	}
	return networkID
}

func handleCoinGeckoWebSocketMessages(conn *websocket.Conn, config *Config) {
	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[COINGECKO] WebSocket read error: %v", err)
			return
		}

		receiveTime := time.Now().UTC()

		// DEBUG: Print raw message to verify structure
		fmt.Printf("\n[DEBUG COINGECKO] Raw message: %s\n", string(messageBytes))

		var trade TradeData
		if err := json.Unmarshal(messageBytes, &trade); err != nil {
			continue
		}

		if trade.Tx == "" || trade.N == "" {
			continue
		}

		lagMs := calculateCoinGeckoLag(trade.T, receiveTime)

		chainName := getChainNameForCoinGecko(trade.N)
		timestamp := receiveTime.Format("2006-01-02 15:04:05")

		tradeTime := time.UnixMilli(trade.T).Format("15:04:05.000")

		fmt.Printf("\n[DEBUG] Raw timestamp: %d | Trade time parsed: %s | Receive time: %s | Lag: %dms\n",
			trade.T, tradeTime, timestamp, lagMs)

		txHashShort := trade.Tx
		if len(txHashShort) > 8 {
			txHashShort = txHashShort[:8]
		}

		tradeType := "buy"
		if trade.Ty == "s" {
			tradeType = "sell"
		}

		fmt.Printf("[COINGECKO][%s][%s] New trade! Tx FULL: %s | Tx: %s... | Type: %s | Volume: $%.2f | Trade time: %s | Lag: %dms\n",
			timestamp,
			chainName,
			trade.Tx,
			txHashShort,
			tradeType,
			trade.Vo,
			tradeTime,
			lagMs,
		)

		RecordLatency("coingecko", chainName, float64(lagMs))
	}
}

func runGeckoTerminalMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting CoinGecko WebSocket monitor...")
	fmt.Printf("Monitoring %d chains with real-time WebSocket\n", len(coinGeckoChains))
	fmt.Printf("Measuring indexation lag (WebSocket push timing)\n")
	fmt.Println()

	if config.CoinGeckoAPIKey == "" {
		fmt.Println("COINGECKO_API_KEY not set in .env file. Skipping CoinGecko monitor.")
		return
	}

	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 60 * time.Second

	for {
		select {
		case <-stopChan:
			fmt.Println("CoinGecko monitor stopped")
			return
		default:
			conn, err := connectCoinGeckoWebSocket(config.CoinGeckoAPIKey)
			if err != nil {
				log.Printf("[COINGECKO] Failed to connect: %v. Retrying in %v...", err, reconnectDelay)
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println("Connected to CoinGecko WebSocket")

			if err := subscribeToCoinGeckoChannel(conn); err != nil {
				log.Printf("[COINGECKO] Failed to subscribe to channel: %v. Retrying in %v...", err, reconnectDelay)
				conn.Close()
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}
			fmt.Println("Subscribed to OnchainTrade channel")

			time.Sleep(500 * time.Millisecond)

			var pools []string
			for _, chain := range coinGeckoChains {
				poolAddress := fmt.Sprintf("%s:%s", chain.networkID, chain.poolAddress)
				pools = append(pools, poolAddress)
			}

			if err := setPoolsForCoinGecko(conn, pools); err != nil {
				log.Printf("[COINGECKO] Failed to set pools: %v. Retrying in %v...", err, reconnectDelay)
				conn.Close()
				time.Sleep(reconnectDelay)
				reconnectDelay = reconnectDelay * 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			}

			fmt.Println("Configured pools for monitoring:")
			for _, chain := range coinGeckoChains {
				fmt.Printf("     - %s (%s)\n", chain.chainName, chain.poolAddress)
			}
			fmt.Println()

			// Reset reconnect delay on successful connection
			reconnectDelay = 5 * time.Second

			// This will block until connection error or stopChan
			handleCoinGeckoWebSocketMessages(conn, config)
			conn.Close()

			// Connection died, log and reconnect
			log.Printf("[COINGECKO] Connection lost. Reconnecting in %v...", reconnectDelay)
			time.Sleep(reconnectDelay)
		}
	}
}
