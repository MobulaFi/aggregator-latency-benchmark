package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ScrapeDefinedSessionCookie visits Defined.fi anonymously and retrieves the session cookie
func ScrapeDefinedSessionCookie() (string, error) {
	// Create Chrome context with headless mode
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Set timeout
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var sessionCookie string

	err := chromedp.Run(ctx,
		// Navigate to Defined.fi
		chromedp.Navigate("https://www.defined.fi/"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),

		// Wait a bit for cookies to be set
		chromedp.Sleep(3*time.Second),

		// Extract session cookie
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookieParams, err := network.GetCookies().Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to get cookies: %w", err)
			}

			for _, cookie := range cookieParams {
				if cookie.Name == "session" {
					sessionCookie = cookie.Value
					fmt.Printf("[SESSION-SCRAPER] Found session cookie (length: %d)\n", len(cookie.Value))
					return nil
				}
			}

			return fmt.Errorf("session cookie not found in %d cookies", len(cookieParams))
		}),
	)

	if err != nil {
		return "", fmt.Errorf("failed to scrape session cookie: %w", err)
	}

	if sessionCookie == "" {
		return "", fmt.Errorf("session cookie is empty")
	}

	return sessionCookie, nil
}

// RefreshSessionCookie scrapes a new session cookie and updates the environment
func RefreshSessionCookie() (string, error) {
	fmt.Println("[SESSION-SCRAPER] Attempting to refresh Defined.fi session cookie...")

	sessionCookie, err := ScrapeDefinedSessionCookie()
	if err != nil {
		return "", fmt.Errorf("failed to refresh session cookie: %w", err)
	}

	// Update environment variable
	os.Setenv("DEFINED_SESSION_COOKIE", sessionCookie)

	fmt.Printf("[SESSION-SCRAPER] ✓ Session cookie refreshed successfully (length: %d)\n", len(sessionCookie))

	return sessionCookie, nil
}

// InvalidateTokenCache forces a token refresh on next request
func InvalidateTokenCache() {
	globalTokenCache.mu.Lock()
	defer globalTokenCache.mu.Unlock()

	globalTokenCache.token = ""
	globalTokenCache.expiresAt = time.Time{}
	fmt.Println("[DEFINED-AUTH] Token cache invalidated")
}
