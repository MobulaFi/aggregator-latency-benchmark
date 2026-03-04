package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	sessionCookie := os.Getenv("DEFINED_SESSION_COOKIE")
	if sessionCookie == "" {
		sessionCookie = "eyJhbGciOiJIUzI1NiJ9.eyJleHBpcmVzQXQiOjE3NzI2NTE0NTYwNjcsImlhdCI6MTc3MjA0NjY1NiwiZXhwIjoxNzcyNjUxNDU2fQ.WvAMpBjBVGiKKd2va9VUPoMHTpNPh2dwzLMwUwW01L0"
	}

	fmt.Println("🔍 Checking JWT token cache...")
	
	token, err := GetDefinedJWTToken(sessionCookie)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Decode expiration
	expiresAt, err := decodeJWTExpiration(token)
	if err != nil {
		fmt.Printf("✅ Token generated (can't decode expiration)\n")
		fmt.Printf("Token (first 50 chars): %s...\n", token[:50])
		os.Exit(0)
	}

	timeUntilExpiry := time.Until(expiresAt)
	fmt.Printf("✅ Token valid!\n")
	fmt.Printf("   Expires in: %.1f hours\n", timeUntilExpiry.Hours())
	fmt.Printf("   Expires at: %s\n", expiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("   Token (first 50 chars): %s...\n", token[:50])
}
