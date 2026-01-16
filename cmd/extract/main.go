package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/DreamCats/bcindex/internal/ast"
)

func main() {
	var (
		root    string
		verbose bool
		stats   bool
	)

	flag.StringVar(&root, "root", ".", "Repository root path")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.BoolVar(&stats, "stats", false, "Show statistics only")
	flag.Parse()

	// Get absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("Failed to resolve root path: %v", err)
	}

	// Check if root exists
	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		log.Fatalf("Repository root does not exist: %s", absRoot)
	}

	fmt.Printf("Analyzing repository: %s\n\n", absRoot)

	pipeline := ast.NewPipeline()

	if stats {
		// Show statistics only
		repoStats, err := pipeline.AnalyzeRepository(absRoot)
		if err != nil {
			log.Fatalf("Failed to analyze repository: %v", err)
		}

		fmt.Printf("Package Count:  %d\n", repoStats.PackageCount)
		fmt.Printf("File Count:      %d\n", repoStats.FileCount)
		fmt.Printf("Symbol Count:    %d\n", repoStats.SymbolCount)
		fmt.Printf("Exported Count:  %d\n", repoStats.ExportedCount)

		if len(repoStats.Errors) > 0 {
			fmt.Printf("\nErrors (%d):\n", len(repoStats.Errors))
			for _, e := range repoStats.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		return
	}

	// Extract all symbols
	symbols, err := pipeline.ExtractRepository(absRoot)
	if err != nil {
		log.Fatalf("Failed to extract repository: %v", err)
	}

	fmt.Printf("Extracted %d symbols\n\n", len(symbols))

	// Group symbols by package
	packages := make(map[string][]*ast.ExtractedSymbol)
	for _, sym := range symbols {
		packages[sym.PackagePath] = append(packages[sym.PackagePath], sym)
	}

	// Print summary
	for pkgPath, pkgSymbols := range packages {
		fmt.Printf("Package: %s (%d symbols)\n", pkgPath, len(pkgSymbols))

		if verbose {
			for _, sym := range pkgSymbols {
				visibility := " "
				if sym.Exported {
					visibility = "✓"
				}
				fmt.Printf("  [%s] %s: %s\n", visibility, sym.Kind, sym.Name)

				if sym.DocComment != "" {
					fmt.Printf("      %s\n", sym.DocComment)
				}
			}
			fmt.Println()
		}
	}

	// Show overall statistics
	var exportedCount int
	for _, sym := range symbols {
		if sym.Exported {
			exportedCount++
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Packages:  %d\n", len(packages))
	fmt.Printf("  Total Symbols:   %d\n", len(symbols))
	fmt.Printf("  Exported:        %d\n", exportedCount)
}
