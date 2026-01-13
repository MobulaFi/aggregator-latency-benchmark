package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ============================================================================
// Metadata Coverage Monitor
// Measures metadata and logo coverage across providers (Mobula, Codex)
// ============================================================================

const (
	mobulaTokenDetailsURL = "https://api.mobula.io/api/2/token/details"
	codexGraphQLURL       = "https://graph.codex.io/graphql"
)

// TokenToCheck represents a token discovered via Pulse that needs metadata checking
type TokenToCheck struct {
	Address    string
	ChainID    string // e.g., "solana", "evm:1", "evm:8453"
	Symbol     string
	Name       string
	DetectedAt time.Time
}

// MetadataFields represents the fields we check for coverage
type MetadataFields struct {
	HasLogo        bool
	HasName        bool
	HasSymbol      bool
	HasDescription bool
	HasTwitter     bool
	HasWebsite     bool
	HasTelegram    bool
	LogoURL        string
	ResponseTimeMs float64
	Error          string
}

// ProviderCoverage holds coverage stats for a single provider
type ProviderCoverage struct {
	Provider       string
	TotalChecks    int
	LogoCount      int
	NameCount      int
	SymbolCount    int
	DescCount      int
	TwitterCount   int
	WebsiteCount   int
	TelegramCount  int
	ErrorCount     int
	TotalLatencyMs float64
}

// MetadataCoverageStats holds overall stats
type MetadataCoverageStats struct {
	mu        sync.Mutex
	Mobula    ProviderCoverage
	Codex     ProviderCoverage
	LastPrint time.Time
}

var (
	coverageStats = &MetadataCoverageStats{
		Mobula: ProviderCoverage{Provider: "mobula"},
		Codex:  ProviderCoverage{Provider: "codex"},
	}
	tokenQueue     = make(chan TokenToCheck, 100)
	metadataClient = &http.Client{Timeout: 10 * time.Second}
)

// ============================================================================
// Mobula API - Token Details
// ============================================================================

type MobulaTokenDetailsResponse struct {
	Data MobulaTokenData `json:"data"`
}

type MobulaTokenData struct {
	Address     string        `json:"address"`
	Name        string        `json:"name"`
	Symbol      string        `json:"symbol"`
	Logo        string        `json:"logo"`
	Description string        `json:"description"`
	Socials     MobulaSocials `json:"socials"`
}

type MobulaSocials struct {
	Twitter  string `json:"twitter"`
	Website  string `json:"website"`
	Telegram string `json:"telegram"`
}

func checkMobulaMetadata(token TokenToCheck, apiKey string) MetadataFields {
	result := MetadataFields{}

	// Build URL with query params
	params := url.Values{}
	params.Add("address", token.Address)

	// Convert chainID to Mobula format
	blockchain := token.ChainID
	if blockchain == "solana:solana" {
		blockchain = "solana"
	}
	params.Add("blockchain", blockchain)

	fullURL := fmt.Sprintf("%s?%s", mobulaTokenDetailsURL, params.Encode())

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("request_create_error: %v", err)
		return result
	}

	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}

	startTime := time.Now()
	resp, err := metadataClient.Do(req)
	result.ResponseTimeMs = float64(time.Since(startTime).Milliseconds())

	if err != nil {
		result.Error = fmt.Sprintf("request_error: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("status_%d", resp.StatusCode)
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read_error: %v", err)
		return result
	}

	var response MobulaTokenDetailsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		result.Error = fmt.Sprintf("parse_error: %v", err)
		return result
	}

	data := response.Data

	// Check each field
	result.HasName = data.Name != ""
	result.HasSymbol = data.Symbol != ""
	result.HasLogo = data.Logo != ""
	result.LogoURL = data.Logo
	result.HasDescription = data.Description != ""
	result.HasTwitter = data.Socials.Twitter != ""
	result.HasWebsite = data.Socials.Website != ""
	result.HasTelegram = data.Socials.Telegram != ""

	return result
}

// ============================================================================
// Codex API - GraphQL Token Query
// ============================================================================

// Note: CodexGraphQLRequest is defined in codex_rest_monitor.go

// CodexTokenResponse represents the response from token query
// Returns EnhancedToken with socialLinks and info
// https://docs.codex.io/api-reference/queries/token
type CodexTokenResponse struct {
	Data struct {
		Token CodexEnhancedToken `json:"token"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// CodexEnhancedToken matches the EnhancedToken type from Codex API
type CodexEnhancedToken struct {
	Address     string           `json:"address"`
	Name        string           `json:"name"`
	Symbol      string           `json:"symbol"`
	Decimals    int              `json:"decimals"`
	NetworkID   int              `json:"networkId"`
	Info        *CodexTokenInfo  `json:"info"`
	SocialLinks *CodexSocialLinks `json:"socialLinks"`
}

// CodexTokenInfo contains metadata about the token
type CodexTokenInfo struct {
	ImageThumbUrl     string `json:"imageThumbUrl"`
	ImageSmallUrl     string `json:"imageSmallUrl"`
	ImageLargeUrl     string `json:"imageLargeUrl"`
	Description       string `json:"description"`
	CirculatingSupply string `json:"circulatingSupply"`
	TotalSupply       string `json:"totalSupply"`
}

// CodexSocialLinks contains social media links for the token
type CodexSocialLinks struct {
	Twitter   string `json:"twitter"`
	Website   string `json:"website"`
	Telegram  string `json:"telegram"`
	Discord   string `json:"discord"`
	Github    string `json:"github"`
}

func getCodexNetworkID(chainID string) int {
	switch chainID {
	case "solana", "solana:solana":
		return 1399811149
	case "evm:1":
		return 1
	case "evm:8453":
		return 8453
	case "evm:56":
		return 56
	case "evm:42161":
		return 42161
	default:
		return 0
	}
}

func checkCodexMetadata(token TokenToCheck, apiKey string) MetadataFields {
	result := MetadataFields{}

	networkID := getCodexNetworkID(token.ChainID)
	if networkID == 0 {
		result.Error = "unsupported_chain"
		return result
	}

	// Use token query which returns EnhancedToken with socialLinks and info
	// https://docs.codex.io/api-reference/queries/token
	query := `query GetToken($address: String!, $networkId: Int!) {
		token(input: { address: $address, networkId: $networkId }) {
			address
			name
			symbol
			decimals
			networkId
			info {
				imageThumbUrl
				imageSmallUrl
				imageLargeUrl
				description
				circulatingSupply
				totalSupply
			}
			socialLinks {
				twitter
				website
				telegram
				discord
				github
			}
		}
	}`

	reqBody := CodexGraphQLRequest{
		Query: query,
		Variables: map[string]interface{}{
			"address":   token.Address,
			"networkId": networkID,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		result.Error = fmt.Sprintf("marshal_error: %v", err)
		return result
	}

	req, err := http.NewRequest("POST", codexGraphQLURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		result.Error = fmt.Sprintf("request_create_error: %v", err)
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}

	startTime := time.Now()
	resp, err := metadataClient.Do(req)
	result.ResponseTimeMs = float64(time.Since(startTime).Milliseconds())

	if err != nil {
		result.Error = fmt.Sprintf("request_error: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		result.Error = fmt.Sprintf("status_%d", resp.StatusCode)
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read_error: %v", err)
		return result
	}

	var response CodexTokenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		result.Error = fmt.Sprintf("parse_error: %v", err)
		return result
	}

	if len(response.Errors) > 0 {
		result.Error = fmt.Sprintf("graphql_error: %s", response.Errors[0].Message)
		return result
	}

	data := response.Data.Token

	// Check if token was found
	if data.Address == "" {
		result.Error = "token_not_found"
		return result
	}

	// Check each field based on EnhancedToken
	// https://docs.codex.io/api-reference/queries/token
	result.HasName = data.Name != ""
	result.HasSymbol = data.Symbol != ""

	// Check logo from info
	if data.Info != nil {
		result.HasLogo = data.Info.ImageThumbUrl != "" || data.Info.ImageSmallUrl != "" || data.Info.ImageLargeUrl != ""
		if data.Info.ImageLargeUrl != "" {
			result.LogoURL = data.Info.ImageLargeUrl
		} else if data.Info.ImageSmallUrl != "" {
			result.LogoURL = data.Info.ImageSmallUrl
		} else {
			result.LogoURL = data.Info.ImageThumbUrl
		}
		result.HasDescription = data.Info.Description != ""
	}

	// Check social links
	if data.SocialLinks != nil {
		result.HasTwitter = data.SocialLinks.Twitter != ""
		result.HasWebsite = data.SocialLinks.Website != ""
		result.HasTelegram = data.SocialLinks.Telegram != ""
	}

	return result
}

// ============================================================================
// Stats and Reporting
// ============================================================================

func updateStats(provider string, fields MetadataFields) {
	coverageStats.mu.Lock()
	defer coverageStats.mu.Unlock()

	var stats *ProviderCoverage
	if provider == "mobula" {
		stats = &coverageStats.Mobula
	} else {
		stats = &coverageStats.Codex
	}

	stats.TotalChecks++
	stats.TotalLatencyMs += fields.ResponseTimeMs

	if fields.Error != "" {
		stats.ErrorCount++
		return
	}

	if fields.HasLogo {
		stats.LogoCount++
	}
	if fields.HasName {
		stats.NameCount++
	}
	if fields.HasSymbol {
		stats.SymbolCount++
	}
	if fields.HasDescription {
		stats.DescCount++
	}
	if fields.HasTwitter {
		stats.TwitterCount++
	}
	if fields.HasWebsite {
		stats.WebsiteCount++
	}
	if fields.HasTelegram {
		stats.TelegramCount++
	}
}

func printCoverageStats() {
	coverageStats.mu.Lock()
	defer coverageStats.mu.Unlock()

	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")

	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                    METADATA COVERAGE STATS - %s                   ║\n", timestamp)
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Provider │ Checks │ Logo  │ Name  │ Symbol│ Desc  │Twitter│Website│Telegram│ Errors │\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")

	for _, stats := range []*ProviderCoverage{&coverageStats.Mobula, &coverageStats.Codex} {
		if stats.TotalChecks == 0 {
			fmt.Printf("║ %-8s │ %6d │   -   │   -   │   -   │   -   │   -   │   -   │   -    │ %6d ║\n",
				stats.Provider, stats.TotalChecks, stats.ErrorCount)
			continue
		}

		successChecks := stats.TotalChecks - stats.ErrorCount
		if successChecks == 0 {
			successChecks = 1 // Avoid division by zero
		}

		fmt.Printf("║ %-8s │ %6d │ %5.1f%%│ %5.1f%%│ %5.1f%%│ %5.1f%%│ %5.1f%%│ %5.1f%%│ %5.1f%% │ %6d ║\n",
			stats.Provider,
			stats.TotalChecks,
			float64(stats.LogoCount)/float64(successChecks)*100,
			float64(stats.NameCount)/float64(successChecks)*100,
			float64(stats.SymbolCount)/float64(successChecks)*100,
			float64(stats.DescCount)/float64(successChecks)*100,
			float64(stats.TwitterCount)/float64(successChecks)*100,
			float64(stats.WebsiteCount)/float64(successChecks)*100,
			float64(stats.TelegramCount)/float64(successChecks)*100,
			stats.ErrorCount,
		)
	}

	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")

	coverageStats.LastPrint = time.Now()
}

func checkTokenMetadata(token TokenToCheck, config *Config) {
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	chainName := getChainNameForPulse(token.ChainID)

	fmt.Printf("\n[METADATA][%s] Checking metadata for new token: %s (%s) on %s\n",
		timestamp, token.Symbol, token.Address[:16]+"...", chainName)

	// Check Mobula
	mobulaResult := checkMobulaMetadata(token, config.MobulaAPIKey)
	updateStats("mobula", mobulaResult)

	mobulaStatus := "✓"
	if mobulaResult.Error != "" {
		mobulaStatus = "✗"
	}
	logoStatus := "✗"
	if mobulaResult.HasLogo {
		logoStatus = "✓"
	}
	descStatus := "✗"
	if mobulaResult.HasDescription {
		descStatus = "✓"
	}
	twitterStatus := "✗"
	if mobulaResult.HasTwitter {
		twitterStatus = "✓"
	}

	fmt.Printf("   [MOBULA] %s | Logo: %s | Desc: %s | Twitter: %s | Latency: %.0fms",
		mobulaStatus, logoStatus, descStatus, twitterStatus, mobulaResult.ResponseTimeMs)
	if mobulaResult.Error != "" {
		fmt.Printf(" | Error: %s", mobulaResult.Error)
	}
	fmt.Printf("\n")

	// Record Prometheus metrics for Mobula
	RecordMetadataCoverage("mobula", chainName, "logo", mobulaResult.HasLogo)
	RecordMetadataCoverage("mobula", chainName, "description", mobulaResult.HasDescription)
	RecordMetadataCoverage("mobula", chainName, "twitter", mobulaResult.HasTwitter)
	RecordMetadataCoverage("mobula", chainName, "website", mobulaResult.HasWebsite)
	RecordMetadataLatency("mobula", chainName, mobulaResult.ResponseTimeMs)

	// Check Codex
	codexResult := checkCodexMetadata(token, config.CodexAPIKey)
	updateStats("codex", codexResult)

	codexStatus := "✓"
	if codexResult.Error != "" {
		codexStatus = "✗"
	}
	logoStatus = "✗"
	if codexResult.HasLogo {
		logoStatus = "✓"
	}
	descStatus = "✗"
	if codexResult.HasDescription {
		descStatus = "✓"
	}

	twitterStatus = "✗"
	if codexResult.HasTwitter {
		twitterStatus = "✓"
	}

	fmt.Printf("   [CODEX]  %s | Logo: %s | Desc: %s | Twitter: %s | Latency: %.0fms",
		codexStatus, logoStatus, descStatus, twitterStatus, codexResult.ResponseTimeMs)
	if codexResult.Error != "" {
		fmt.Printf(" | Error: %s", codexResult.Error)
	}
	fmt.Printf("\n")

	// Record Prometheus metrics for Codex
	RecordMetadataCoverage("codex", chainName, "logo", codexResult.HasLogo)
	RecordMetadataCoverage("codex", chainName, "description", codexResult.HasDescription)
	RecordMetadataCoverage("codex", chainName, "twitter", codexResult.HasTwitter)
	RecordMetadataCoverage("codex", chainName, "website", codexResult.HasWebsite)
	RecordMetadataLatency("codex", chainName, codexResult.ResponseTimeMs)

	// Print stats every 10 checks
	coverageStats.mu.Lock()
	totalChecks := coverageStats.Mobula.TotalChecks
	coverageStats.mu.Unlock()

	if totalChecks > 0 && totalChecks%10 == 0 {
		printCoverageStats()
	}
}

// QueueTokenForMetadataCheck adds a token to the check queue
func QueueTokenForMetadataCheck(token TokenToCheck) {
	select {
	case tokenQueue <- token:
		// Token queued successfully
	default:
		// Queue full, skip this token
		fmt.Printf("[METADATA] Queue full, skipping token: %s\n", token.Address)
	}
}

// runMetadataCoverageMonitor starts the metadata coverage monitoring
func runMetadataCoverageMonitor(config *Config, stopChan <-chan struct{}) {
	fmt.Println("Starting Metadata Coverage Monitor...")
	fmt.Println("   Comparing metadata coverage: Mobula vs Codex")
	fmt.Println("   Fields tracked: Logo, Name, Symbol, Description, Twitter, Website, Telegram")
	fmt.Println("   Waiting for new tokens from Pulse stream...")
	fmt.Println()

	// Stats printer ticker - print every 5 minutes
	statsTicker := time.NewTicker(5 * time.Minute)
	defer statsTicker.Stop()

	for {
		select {
		case <-stopChan:
			fmt.Println("Metadata Coverage monitor stopped")
			printCoverageStats() // Print final stats
			return

		case token := <-tokenQueue:
			// Small delay to let the token get indexed
			time.Sleep(2 * time.Second)
			checkTokenMetadata(token, config)

		case <-statsTicker.C:
			printCoverageStats()
		}
	}
}

