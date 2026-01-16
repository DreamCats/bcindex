package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/DreamCats/bcindex/internal/ast"
)

func main() {
	var (
		root    string
		verbose bool
	)

	flag.StringVar(&root, "root", ".", "Repository root path")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.Parse()

	// Get absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("Failed to resolve root path: %v", err)
	}

	fmt.Printf("Analyzing repository: %s\n\n", absRoot)

	pipeline := ast.NewPipeline()

	// Extract symbols and relations
	symbols, edges, err := pipeline.ExtractRepositoryWithRelations(absRoot)
	if err != nil {
		log.Fatalf("Failed to extract repository: %v", err)
	}

	fmt.Printf("Extracted %d symbols\n", len(symbols))
	fmt.Printf("Found %d relationships\n\n", len(edges))

	// Group edges by type
	edgeTypes := make(map[string]int)
	for _, edge := range edges {
		edgeTypes[edge.EdgeType]++
	}

	fmt.Println("Relationship types:")
	for edgeType, count := range edgeTypes {
		fmt.Printf("  %s: %d\n", edgeType, count)
	}

	if verbose {
		fmt.Println("\nRelationships:")
		for _, edge := range edges {
			fromSym := findSymbol(symbols, edge.FromID)
			toSym := findSymbol(symbols, edge.ToID)

			fromName := edge.FromID
			toName := edge.ToID

			if fromSym != nil {
				fromName = fmt.Sprintf("%s/%s", fromSym.PackageName, fromSym.Name)
			}
			if toSym != nil {
				toName = fmt.Sprintf("%s/%s", toSym.PackageName, toSym.Name)
			}

			fmt.Printf("  [%s] %s -> %s (weight: %d)\n", edge.EdgeType, fromName, toName, edge.Weight)
			if edge.ImportPath != "" {
				fmt.Printf("      import: %s\n", edge.ImportPath)
			}
		}
	}

	// Show summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Symbols:   %d\n", len(symbols))
	fmt.Printf("  Total Edges:     %d\n", len(edges))
	fmt.Printf("  Edge Types:      %d\n", len(edgeTypes))
}

func findSymbol(symbols []*ast.ExtractedSymbol, id string) *ast.ExtractedSymbol {
	for _, sym := range symbols {
		if sym.ID == id {
			return sym
		}
	}
	return nil
}
