// +build ignore

package main

import (
	"fmt"
)

func main() {
	fmt.Println("Testing Defined.fi session cookie scraper (anonymous mode)...")
	fmt.Println()

	sessionCookie, err := ScrapeDefinedSessionCookie()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✓ Success!")
	fmt.Printf("Session Cookie: %s...\n", sessionCookie[:min(50, len(sessionCookie))])
	fmt.Printf("Full length: %d characters\n", len(sessionCookie))
}
