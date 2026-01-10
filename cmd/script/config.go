package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	CoinGeckoAPIKey string
	MobulaAPIKey    string
	CodexAPIKey     string
}

func loadEnv() (*Config, error) {
	config := &Config{}

	// First, try to load from environment variables (for production/Railway)
	config.CoinGeckoAPIKey = os.Getenv("COINGECKO_API_KEY")
	config.MobulaAPIKey = os.Getenv("MOBULA_API_KEY")
	config.CodexAPIKey = os.Getenv("CODEX_API_KEY")

	// If all env vars are set, return early (production mode)
	if config.CoinGeckoAPIKey != "" || config.MobulaAPIKey != "" || config.CodexAPIKey != "" {
		return config, nil
	}

	// Otherwise, try to load from .env file (for local development)
	file, err := os.Open(".env")
	if err != nil {
		// If no .env file and no env vars, that's OK - services will just be skipped
		return config, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		switch key {
		case "COINGECKO_API_KEY":
			if config.CoinGeckoAPIKey == "" {
				config.CoinGeckoAPIKey = value
			}
		case "MOBULA_API_KEY":
			if config.MobulaAPIKey == "" {
				config.MobulaAPIKey = value
			}
		case "CODEX_API_KEY":
			if config.CodexAPIKey == "" {
				config.CodexAPIKey = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %w", err)
	}

	return config, nil
}
