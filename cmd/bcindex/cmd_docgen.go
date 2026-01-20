package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DreamCats/bcindex/cmd/bcindex/internal"
	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/docgen"
)

// handleDocGen implements the docgen subcommand
func handleDocGen(cfg *config.Config, repoRoot string, args []string) {
	fs := flag.NewFlagSet("docgen", flag.ExitOnError)

	var dryRun, diff, overwrite, verbose bool
	var maxPerFile, maxTotal, concurrency int
	var includeList, excludeList internal.StringList

	fs.BoolVar(&dryRun, "dry-run", false, "Only scan and generate, don't write to files")
	fs.BoolVar(&diff, "diff", false, "Output unified diff of changes")
	fs.BoolVar(&overwrite, "overwrite", false, "Overwrite existing documentation")
	fs.BoolVar(&verbose, "v", false, "Verbose output (show generation errors)")
	fs.IntVar(&maxPerFile, "max-per-file", 50, "Maximum symbols to process per file")
	fs.IntVar(&maxTotal, "max", 200, "Maximum total symbols to process")
	fs.IntVar(&concurrency, "concurrency", 4, "Number of concurrent LLM requests")
	fs.Var(&includeList, "include", "Include paths (can be specified multiple times)")
	fs.Var(&excludeList, "exclude", "Exclude paths (can be specified multiple times)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex docgen [options]

DESCRIPTION:
    Generate documentation for Go code using LLM.
    This command scans for symbols missing documentation and generates
    appropriate doc comments following Go conventions.

    The generated comments follow these principles:
    - First sentence starts with the symbol name
    - Concise: one sentence summary + optional key constraints/errors
    - Chinese for explanation + English for technical terms
    - No implementation details

OPTIONS:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
EXAMPLES:
    # Dry run to see what would be documented
    bcindex docgen --dry-run

    # Show diff of changes
    bcindex docgen --diff

    # Generate with limits
    bcindex docgen --max 100 --max-per-file 20

    # Only include specific paths
    bcindex docgen --include internal/service --include internal/handler

    # Exclude test and vendor directories
    bcindex docgen --exclude vendor --exclude testdata

    # Higher concurrency for faster processing
    bcindex docgen --concurrency 8

NOTES:
    - Requires docgen.api_key or embedding.api_key in config
    - Default model: doubao-1-5-pro-32k-250115
    - Use --dry-run first to preview changes before applying
`)
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}

	// Build scanner options
	var scannerOpts []docgen.Option
	scannerOpts = append(scannerOpts,
		docgen.WithInclude(includeList...),
		docgen.WithExclude(excludeList...),
		docgen.WithSkipTests(true),
		docgen.WithMaxPerFile(maxPerFile),
		docgen.WithMaxTotal(maxTotal),
	)

	// Build writer options
	var writerOpts []docgen.WriterOption
	writerOpts = append(writerOpts,
		docgen.WithDryRun(dryRun),
		docgen.WithDiff(diff),
		docgen.WithVerbose(verbose),
	)

	fmt.Printf("🔍 Scanning for symbols without documentation...\n\n")

	// Scan for symbols needing documentation
	scanner := docgen.NewScanner(repoRoot, scannerOpts...)
	ctx := context.Background()

	scanResults, err := scanner.Scan(ctx)
	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	if len(scanResults) == 0 {
		fmt.Println("✅ No symbols found missing documentation!")
		return
	}

	fmt.Printf("Found %d symbols needing documentation:\n", len(scanResults))

	// Group by file
	byFile := make(map[string][]docgen.ScanResult)
	for _, r := range scanResults {
		byFile[r.File] = append(byFile[r.File], r)
	}

	fileCount := len(byFile)
	fmt.Printf("  Files: %d\n", fileCount)
	fmt.Printf("  Symbols: %d\n\n", len(scanResults))

	// Convert scan results to symbol info for LLM
	symbols := make([]docgen.SymbolInfo, 0, len(scanResults))
	for _, r := range scanResults {
		relPath, _ := filepath.Rel(repoRoot, r.File)
		symbols = append(symbols, docgen.SymbolInfo{
			ID:        fmt.Sprintf("%s:%d", relPath, r.StartLine),
			Name:      r.SymbolName,
			Kind:      r.SymbolKind,
			Signature: r.Signature,
			Package:   r.Package,
			FilePath:  relPath,
			Line:      r.StartLine,
			Receiver:  r.Receiver,
		})
	}

	// Create generator
	fmt.Println("🤖 Generating documentation...")
	gen, err := docgen.NewGenerator(&cfg.DocGen)
	if err != nil {
		// Try falling back to embedding config
		if cfg.Embedding.APIKey != "" {
			gen, err = docgen.NewGenerator(&config.DocGenConfig{
				APIKey:   cfg.Embedding.APIKey,
				Endpoint: cfg.DocGen.Endpoint,
				Model:    cfg.DocGen.Model,
			})
			if err != nil {
				log.Fatalf("Failed to create generator: %v", err)
			}
		} else {
			log.Fatalf("Failed to create generator: %v (configure docgen.api_key or embedding.api_key)", err)
		}
	}

	// Generate documentation in batches with concurrency
	const batchSize = 10
	type batchResult struct {
		start   int
		end     int
		results []docgen.GenerateResult
		err     error
	}

	var batchResults []batchResult
	var mu sync.Mutex

	// Create a semaphore to limit concurrency
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	var batchIndices []int

	for i := 0; i < len(symbols); i += batchSize {
		batchIndices = append(batchIndices, i)
	}

	// Process batches concurrently
	for _, batchStart := range batchIndices {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			end := start + batchSize
			if end > len(symbols) {
				end = len(symbols)
			}

			batch := symbols[start:end]
			results, err := gen.GenerateBatch(ctx, batch)

			mu.Lock()
			batchResults = append(batchResults, batchResult{
				start:   start,
				end:     end,
				results: results,
				err:     err,
			})
			// Print progress immediately
			if err != nil {
				fmt.Printf("  [%d-%d] Failed: %v\n", start, end, err)
			} else {
				fmt.Printf("  [%d-%d] Generated %d/%d\n", start, end-1, end, len(symbols))
			}
			mu.Unlock()
		}(batchStart)
	}

	wg.Wait()

	// Sort batch results by start position and collect all results
	for i := 0; i < len(batchResults); i++ {
		for j := i + 1; j < len(batchResults); j++ {
			if batchResults[i].start > batchResults[j].start {
				batchResults[i], batchResults[j] = batchResults[j], batchResults[i]
			}
		}
	}

	var allResults []docgen.GenerateResult
	for _, br := range batchResults {
		if br.err != nil {
			// Add error results for this batch
			for _, sym := range symbols[br.start:br.end] {
				allResults = append(allResults, docgen.GenerateResult{
					ID:    sym.ID,
					Error: "generation failed",
				})
			}
			continue
		}

		allResults = append(allResults, br.results...)
	}

	// Prepare write requests
	var writeRequests []docgen.WriteRequest
	var generationErrors []string
	for i, scan := range scanResults {
		if i >= len(allResults) {
			break
		}
		result := allResults[i]

		if result.Error != "" {
			generationErrors = append(generationErrors, fmt.Sprintf("%s (%s:%d): %s", scan.SymbolName, scan.File, scan.StartLine, result.Error))
			continue
		}

		writeRequests = append(writeRequests, docgen.WriteRequest{
			File:      scan.File,
			Symbol:    scan.SymbolName,
			Line:      scan.StartLine,
			Comment:   result.Comment,
			Overwrite: overwrite,
		})
	}

	// Log generation errors if any
	if len(generationErrors) > 0 && verbose {
		for _, err := range generationErrors {
			log.Printf("Generation error: %s\n", err)
		}
	}

	fmt.Printf("\n✅ Generated %d documentation comments\n", len(writeRequests))

	// Write or show diff
	writer := docgen.NewWriter(writerOpts...)

	if diff {
		fmt.Println("\n📝 Diff of changes:")
		fmt.Println(strings.Repeat("=", 60))
	}

	results := writer.Write(writeRequests)

	// Print summary
	successCount := 0
	errorCount := 0
	modifiedCount := 0
	var writeErrors []string
	for _, r := range results {
		if r.Success {
			successCount++
			if r.Modified {
				modifiedCount++
			}
		} else {
			errorCount++
			writeErrors = append(writeErrors, fmt.Sprintf("  %s:%s - %s", r.File, r.Symbol, r.Error))
		}
		if diff && r.Diff != "" {
			fmt.Printf("\n--- %s:%s ---\n", r.File, r.Symbol)
			fmt.Println(r.Diff)
		}
	}

	if diff {
		fmt.Println(strings.Repeat("=", 60))
	}

	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   Generated: %d (LLM successfully generated documentation)\n", successCount)
	if modifiedCount > 0 {
		fmt.Printf("   Modified:  %d (files would be modified)\n", modifiedCount)
	}
	if errorCount > 0 {
		fmt.Printf("   Errors:    %d\n", errorCount)
		// Always show first few errors
		maxShow := 5
		if len(writeErrors) > maxShow {
			fmt.Println("   Error details (first 5):")
			for _, err := range writeErrors[:maxShow] {
				fmt.Println(err)
			}
			fmt.Printf("   ... and %d more errors\n", len(writeErrors)-maxShow)
		} else {
			fmt.Println("   Error details:")
			for _, err := range writeErrors {
				fmt.Println(err)
			}
		}
	}

	if dryRun {
		fmt.Println("\n⚠️  Dry run mode - no files were modified")
		fmt.Println("    Run without --dry-run to apply changes")
	}
}
