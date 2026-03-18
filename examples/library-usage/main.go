// Example of using smokepod as a Go library
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/peteretelej/smokepod/pkg/smokepod"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: library-usage <config.yaml>")
		os.Exit(1)
	}

	configPath := os.Args[1]

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Run tests from config file
	result, err := smokepod.RunFile(ctx, configPath)
	if err != nil {
		log.Fatalf("Error running tests: %v", err)
	}

	// Print summary
	fmt.Printf("Test Suite: %s\n", result.Name)
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("Passed: %v\n\n", result.Passed)

	fmt.Printf("Summary:\n")
	fmt.Printf("  Total:   %d\n", result.Summary.Total)
	fmt.Printf("  Passed:  %d\n", result.Summary.Passed)
	fmt.Printf("  Failed:  %d\n", result.Summary.Failed)
	fmt.Printf("  Skipped: %d\n", result.Summary.Skipped)
	fmt.Printf("  XFail:   %d\n", result.Summary.XFail)
	fmt.Printf("  XPass:   %d\n\n", result.Summary.XPass)

	// Print details for each test
	fmt.Println("Tests:")
	for _, test := range result.Tests {
		status := "PASS"
		if !test.Passed {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %s (%s)\n", status, test.Name, test.Duration)
		if test.Error != "" {
			fmt.Printf("         Error: %s\n", test.Error)
		}
	}

	// Output full JSON result
	fmt.Println("\nJSON Output:")
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))

	// Exit with appropriate code
	if !result.Passed {
		os.Exit(1)
	}
}
