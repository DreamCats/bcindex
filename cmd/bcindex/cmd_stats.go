package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/indexer"
)

// handleStats implements the stats subcommand
func handleStats(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Output as JSON")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex stats [options]

DESCRIPTION:
    Show statistics about the current index.

OPTIONS:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
    # Show human-readable statistics
    bcindex stats

    # JSON output
    bcindex stats -json
`)
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}

	// Create indexer
	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		log.Fatalf("Failed to create indexer: %v", err)
	}
	defer idx.Close()

	// Get stores and statistics
	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()

	symbolCount, _ := symbolStore.Count()
	packageCount, _ := packageStore.Count()
	edgeCount, _ := edgeStore.Count()
	vectorCount, _ := vectorStore.Count()

	if jsonOutput {
		stats := map[string]interface{}{
			"symbols":    symbolCount,
			"packages":   packageCount,
			"edges":      edgeCount,
			"embeddings": vectorCount,
		}
		jsonData, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("📊 Index Statistics")
		fmt.Println()
		fmt.Printf("Packages:   %6d\n", packageCount)
		fmt.Printf("Symbols:    %6d\n", symbolCount)
		fmt.Printf("Edges:      %6d\n", edgeCount)
		fmt.Printf("Embeddings: %6d\n", vectorCount)
	}
}
