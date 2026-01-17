package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/indexer"
	"github.com/DreamCats/bcindex/internal/retrieval"
)

var version = "1.0.3"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Parse global flags and find subcommand
	configPath := ""
	repoPath := ""
	args := os.Args[1:]

	// Handle special flags that don't require subcommand
	for _, arg := range args {
		if arg == "-h" || arg == "-help" || arg == "--help" {
			printUsage()
			os.Exit(0)
		}
		if arg == "-v" || arg == "-version" || arg == "--version" {
			fmt.Printf("bcindex version %s\n", version)
			os.Exit(0)
		}
	}

	// Find the subcommand (first non-flag argument that is a valid subcommand)
	validSubcommands := map[string]bool{
		"index":    true,
		"search":   true,
		"evidence": true,
		"stats":    true,
	}

	subcommandIndex := -1
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			// Check if this is a known subcommand
			if validSubcommands[arg] {
				subcommandIndex = i
				break
			}
			// Not a known subcommand, might be a value for a flag
		}
	}

	if subcommandIndex == -1 {
		fmt.Fprintf(os.Stderr, "Error: No subcommand specified\n\n")
		printUsage()
		os.Exit(1)
	}

	// Parse global flags (before subcommand)
	globalFlags := args[:subcommandIndex]
	for i := 0; i < len(globalFlags); i++ {
		flag := globalFlags[i]
		if flag == "-config" || flag == "--config" {
			if i+1 < len(globalFlags) {
				configPath = globalFlags[i+1]
				i++ // skip next arg
			}
		} else if flag == "-repo" || flag == "--repo" {
			if i+1 < len(globalFlags) {
				repoPath = globalFlags[i+1]
				i++ // skip next arg
			}
		} else if flag == "-h" || flag == "-help" || flag == "--help" {
			printUsage()
			os.Exit(0)
		} else if flag == "-v" || flag == "-version" || flag == "--version" {
			fmt.Printf("bcindex version %s\n", version)
			os.Exit(0)
		} else if strings.HasPrefix(flag, "-") {
			fmt.Fprintf(os.Stderr, "Error: Unknown global flag: %s\n\n", flag)
			printUsage()
			os.Exit(1)
		}
	}

	// Load configuration
	cfg, err := loadConfig(configPath)
	if err != nil {
		if config.IsConfigNotFound(err) {
			if subcommand := args[subcommandIndex]; subcommand == "index" {
				if notFoundErr, ok := err.(*config.ConfigNotFoundError); ok {
					created, createErr := config.WriteDefaultTemplate(notFoundErr.RequestedPath)
					if createErr != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
						fmt.Fprintf(os.Stderr, "Also failed to create default config at %s: %v\n\n", notFoundErr.RequestedPath, createErr)
						printConfigExample()
						os.Exit(1)
					}
					if created {
						fmt.Fprintf(os.Stderr, "Created default config at %s\n", notFoundErr.RequestedPath)
					}
					fmt.Fprintln(os.Stderr, "Please update embedding.api_key in the config file and rerun `bcindex index`.")
					os.Exit(1)
				}
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			printConfigExample()
			os.Exit(1)
		}
		log.Fatalf("Failed to load config: %v\n", err)
	}

	// Override repo path if specified
	if repoPath != "" {
		cfg.Repo.Path = repoPath
	}

	repoRoot, err := resolveRepoRoot(cfg.Repo.Path)
	if err != nil {
		log.Fatalf("Failed to resolve repository root: %v\n", err)
	}
	cfg.Repo.Path = repoRoot

	dbPath, err := defaultDBPath(repoRoot)
	if err != nil {
		log.Fatalf("Failed to determine database path: %v\n", err)
	}
	cfg.Database.Path = dbPath

	// Execute subcommand
	subcommand := args[subcommandIndex]
	subcommandArgs := args[subcommandIndex+1:]

	if subcommand != "evidence" {
		if err := setupLogging(subcommand, repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize log file: %v\n", err)
		}
	}

	switch subcommand {
	case "index":
		handleIndex(cfg, subcommandArgs)
	case "search":
		handleSearch(cfg, subcommandArgs)
	case "evidence":
		handleEvidence(cfg, subcommandArgs)
	case "stats":
		handleStats(cfg, subcommandArgs)
	default:
		fmt.Printf("Unknown subcommand: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

func setupLogging(subcommand string, repoRoot string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(homeDir, ".bcindex", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	repoName := sanitizeRepoName(filepath.Base(repoRoot))
	hash := sha1.Sum([]byte(repoRoot))
	suffix := hex.EncodeToString(hash[:])[:8]
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("bcindex-%s-%s-%s-%s.log", subcommand, repoName, timestamp, suffix)
	logPath := filepath.Join(logDir, filename)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	log.Printf("Log file: %s", logPath)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `bcindex - Semantic Code Search for Go Projects

Version: %s

USAGE:
    bcindex [global options] <command> [command options]

GLOBAL OPTIONS:
    -config <path>
        Path to config file (default: ~/.bcindex/config/bcindex.yaml)

    -repo <path>
        Override repository path

    -v, -version
        Show version information

    -h, -help
        Show this help message

COMMANDS:
    index
        Build index for a Go repository

    search
        Search for code using natural language or keywords

    evidence
        Search and return LLM-friendly evidence pack (JSON)

    stats
        Show index statistics

EXAMPLES:
    # Index current directory
    bcindex index

    # Index specific repository
    bcindex -repo /path/to/repo index

    # Search for code
    bcindex search "order status change"

    # Search with vector-only mode
    bcindex search "database connection" -vector-only

    # Get evidence pack for LLM
    bcindex evidence "implement idempotent API" -output evidence.json

    # Show statistics
    bcindex stats

For detailed help on each command, use:
    bcindex <command> -help
`, version)
}

func loadConfig(configPath string) (*config.Config, error) {
	if configPath != "" {
		return config.LoadFromFile(configPath)
	}
	return config.Load()
}

// resolveRepoRoot resolves the absolute path of the repository root directory.
// It first converts the relative path to an absolute path.
// If the path is a Git repository, it returns the Git root directory.
// Otherwise, it returns the absolute path.
func resolveRepoRoot(repoPath string) (string, error) {
	root := repoPath
	if root == "" || root == "." {
		root = "."
	}

	absPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	if gitRoot := gitTopLevel(absPath); gitRoot != "" {
		absPath = gitRoot
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}

	return absPath, nil
}

func gitTopLevel(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return ""
	}
	return root
}

func defaultDBPath(repoRoot string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := filepath.Join(homeDir, ".bcindex", "data")
	repoName := sanitizeRepoName(filepath.Base(repoRoot))
	hash := sha1.Sum([]byte(repoRoot))
	suffix := hex.EncodeToString(hash[:])[:12]
	filename := fmt.Sprintf("%s-%s.db", repoName, suffix)
	return filepath.Join(dataDir, filename), nil
}

func sanitizeRepoName(name string) string {
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "repo"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	if b.Len() == 0 {
		return "repo"
	}
	return b.String()
}

func printConfigExample() {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".bcindex", "config", "bcindex.yaml")

	fmt.Fprintf(os.Stderr, `Create a configuration file at %s:

# Embedding service configuration (required)
embedding:
  # Provider: "volcengine" | "openai"
  provider: volcengine

  # VolcEngine configuration
  api_key: your-volcengine-api-key
  endpoint: https://ark.cn-beijing.volces.com/api/v3
  model: doubao-embedding-vision-250615

  # Embedding parameters
  dimensions: 2048              # 1024 or 2048
  batch_size: 10                # Batch size for embedding requests
  encoding_format: float        # "float" or "base64"
 
# Database configuration
# Database is stored per-repository under ~/.bcindex/data/

# For OpenAI provider, use:
# embedding:
#   provider: openai
#   openai_api_key: your-openai-api-key
#   openai_model: text-embedding-3-small
#   dimensions: 1536

Usage:
  1. Create the config file
  2. Navigate to your Go project: cd /path/to/project
  3. Run: bcindex index
  4. Search: bcindex search "your query"
`, configPath)
}

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

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
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

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
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
