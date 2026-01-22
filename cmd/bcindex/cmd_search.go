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

// handleSearch implements the search subcommand
func handleSearch(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	var topK int
	var vectorOnly, keywordOnly, jsonOutput, verbose bool
	var includeUnexported bool

	fs.IntVar(&topK, "k", 10, "Number of results to return")
	fs.BoolVar(&vectorOnly, "vector-only", false, "Use vector search only")
	fs.BoolVar(&keywordOnly, "keyword-only", false, "Use keyword search only")
	fs.BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	fs.BoolVar(&verbose, "v", false, "Verbose output (show scores and reasons)")
	fs.BoolVar(&includeUnexported, "all", false, "Include unexported symbols")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex search [options] "<query>"

DESCRIPTION:
    Search for code using natural language queries.
    Supports hybrid search combining vector similarity and keyword matching.

OPTIONS:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
    # Natural language search
    bcindex search "function to create order"

    # Keyword-only search
    bcindex search "CreateOrder" -keyword-only

    # Get top 20 results
    bcindex search "database connection" -k 20

    # JSON output for scripting
    bcindex search "error handling" -json

    # Verbose output with scores
    bcindex search "order status" -v

    # Include unexported symbols
    bcindex search "outputJSON" -all
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

	// Configure search options
	opts := retrieval.DefaultSearchOptions()
	opts.TopK = topK
	if includeUnexported {
		opts.ExportedOnly = false
	}

	if vectorOnly {
		opts.VectorWeight = 1.0
		opts.KeywordWeight = 0.0
		opts.GraphWeight = 0.0
	} else if keywordOnly {
		opts.VectorWeight = 0.0
		opts.KeywordWeight = 1.0
		opts.GraphWeight = 0.0
	}

	// Perform search
	ctx := context.Background()
	results, err := retriever.Search(ctx, query, opts)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	// Output results
	if jsonOutput {
		outputJSON(results, query)
	} else {
		outputText(results, query, verbose)
	}
}

// outputText outputs search results as human-readable text
func outputText(results []retrieval.SearchResult, query string, verbose bool) {
	if len(results) == 0 {
		fmt.Println("No results found")
		return
	}

	fmt.Printf("Found %d result(s) for: %s\n\n", len(results), query)

	for i, result := range results {
		fmt.Printf("%d. %s\n", i+1, result.Symbol.Name)
		fmt.Printf("   Kind:    %s\n", result.Symbol.Kind)
		fmt.Printf("   Package: %s\n", result.Symbol.PackagePath)
		fmt.Printf("   File:    %s:%d\n", result.Symbol.FilePath, result.Symbol.LineStart)

		if verbose {
			if result.VectorScore > 0 {
				fmt.Printf("   Vector:  %.3f\n", result.VectorScore)
			}
			if result.KeywordScore > 0 {
				fmt.Printf("   Keyword: %.3f\n", result.KeywordScore)
			}
			if result.GraphScore > 0 {
				fmt.Printf("   Graph:   %.3f\n", result.GraphScore)
			}
			fmt.Printf("   Score:   %.3f\n", result.CombinedScore)

			if len(result.Reason) > 0 {
				fmt.Printf("   Why:     %v\n", result.Reason)
			}
		}

		if result.Symbol.SemanticText != "" {
			text := result.Symbol.SemanticText
			if len(text) > 100 {
				text = text[:100] + "..."
			}
			fmt.Printf("   %s\n", text)
		}

		fmt.Println()
	}
}

// outputJSON outputs search results as JSON
func outputJSON(results []retrieval.SearchResult, query string) {
	output := map[string]interface{}{
		"query":   query,
		"count":   len(results),
		"results": results,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal results: %v", err)
	}

	fmt.Println(string(jsonData))
}
