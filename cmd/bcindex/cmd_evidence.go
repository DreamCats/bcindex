package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/indexer"
	"github.com/DreamCats/bcindex/internal/retrieval"
)

// handleEvidence implements the evidence subcommand
func handleEvidence(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("evidence", flag.ExitOnError)

	var outputFile string
	var maxPackages, maxSymbols, maxSnippets, maxLines int

	fs.StringVar(&outputFile, "output", "", "Output file path (default: stdout)")
	fs.IntVar(&maxPackages, "max-packages", 3, "Maximum number of packages to include")
	fs.IntVar(&maxSymbols, "max-symbols", 10, "Maximum number of symbols to include")
	fs.IntVar(&maxSnippets, "max-snippets", 5, "Maximum number of code snippets")
	fs.IntVar(&maxLines, "max-lines", 200, "Maximum total lines across all snippets")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex evidence [options] "<query>"

DESCRIPTION:
    Generate LLM-friendly evidence pack for code search.
    Returns structured JSON with:
      - Package cards with roles and summaries
      - Symbol cards with signatures and reasons
      - Code snippets with strict line control
      - Graph hints showing relationships

    This is designed for Claude Code, Cursor, or other AI assistants.

OPTIONS:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
    # Generate evidence pack to stdout
    bcindex evidence "order status change implementation"

    # Save to file
    bcindex evidence "implement idempotent API" -output evidence.json

    # Get more symbols and snippets
    bcindex evidence "payment processing flow" -max-symbols 20 -max-snippets 10

    # Increase line limit
    bcindex evidence "database migration" -max-lines 500
`)
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}

	// Get query from remaining arguments
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: search query is required\n\n")
		fs.Usage()
		os.Exit(1)
	}

	query := fs.Arg(0)

	// Create indexer and retriever
	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		log.Fatalf("Failed to create indexer: %v", err)
	}
	defer idx.Close()

	expander, err := retrieval.LoadSynonymsForRepo(cfg.Repo.Path, cfg.Search.SynonymsFile)
	if err != nil {
		log.Printf("Warning: failed to load synonyms file: %v", err)
	}

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
		expander,
	)

	// Configure evidence builder
	evidenceBuilder := retriever.GetEvidenceBuilder()
	evidenceBuilder.SetMaxPackages(maxPackages)
	evidenceBuilder.SetMaxSymbols(maxSymbols)
	evidenceBuilder.SetMaxSnippets(maxSnippets)
	evidenceBuilder.SetMaxLines(maxLines)

	// Perform search and build evidence pack
	ctx := context.Background()
	opts := retrieval.DefaultSearchOptions()

	pack, err := retriever.SearchAsEvidencePack(ctx, query, opts)
	if err != nil {
		log.Fatalf("Failed to build evidence pack: %v", err)
	}

	// Output as JSON
	jsonData, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal evidence pack: %v", err)
	}

	if outputFile != "" {
		if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
			log.Fatalf("Failed to write output file: %v", err)
		}
		fmt.Printf("✅ Evidence pack written to: %s\n", outputFile)
		fmt.Printf("\n📊 Summary:\n")
		fmt.Printf("   Query:    %s\n", pack.Query)
		fmt.Printf("   Packages: %d\n", len(pack.TopPackages))
		fmt.Printf("   Symbols:  %d\n", len(pack.TopSymbols))
		fmt.Printf("   Snippets: %d\n", len(pack.Snippets))
		fmt.Printf("   Lines:    %d\n", pack.Metadata.TotalLines)
	} else {
		fmt.Println(string(jsonData))
	}
}
