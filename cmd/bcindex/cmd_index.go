package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/indexer"
)

// handleIndex implements the index subcommand
func handleIndex(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	force := fs.Bool("force", false, "Force rebuild index")
	verbose := fs.Bool("v", false, "Verbose output")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex index [options]

DESCRIPTION:
    Build semantic search index for a Go repository.
    This will:
      1. Parse all Go files using AST
      2. Extract symbols (functions, types, methods, interfaces)
      3. Build call graph and import relationships
      4. Generate semantic descriptions
      5. Create embeddings for vector search

OPTIONS:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
    # Index current directory (must be a Go project)
    bcindex index

    # Index from any directory (using -repo flag)
    bcindex index -repo /path/to/go/project

    # Force rebuild existing index
    bcindex index -force

    # Verbose output
    bcindex index -v
`)
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}

	// Determine repository path
	absPath := cfg.Repo.Path

	// Check if repository exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		log.Fatalf("Repository path does not exist: %s", absPath)
	}

	// Check if it's a Go module (has go.mod)
	if _, err := os.Stat(filepath.Join(absPath, "go.mod")); os.IsNotExist(err) {
		fmt.Printf("⚠️  Warning: No go.mod file found in %s\n", absPath)
		fmt.Printf("    Indexing will proceed but may not work correctly.\n\n")
	}

	fmt.Printf("🏗️  Building index for: %s\n\n", absPath)

	// Create indexer
	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		log.Fatalf("Failed to create indexer: %v", err)
	}
	defer idx.Close()

	// Start indexing
	startTime := time.Now()
	ctx := context.Background()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	if err := idx.IndexRepository(ctx, absPath); err != nil {
		log.Fatalf("Indexing failed: %v", err)
	}

	_ = *force // TODO: implement force rebuild

	duration := time.Since(startTime)

	// Print statistics
	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	symbolCount, _ := symbolStore.Count()
	packageCount, _ := packageStore.Count()
	edgeCount, _ := edgeStore.Count()
	vectorCount, _ := vectorStore.Count()

	fmt.Println()
	fmt.Println("✅ Indexing completed successfully!")
	fmt.Printf("\n⏱️  Duration: %v\n", duration)
	fmt.Println("\n📊 Statistics:")
	fmt.Printf("   Packages:   %6d\n", packageCount)
	fmt.Printf("   Symbols:    %6d\n", symbolCount)
	fmt.Printf("   Relations:  %6d\n", edgeCount)
	fmt.Printf("   Embeddings: %6d\n", vectorCount)
}
