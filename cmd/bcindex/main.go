package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/DreamCats/bcindex/cmd/bcindex/internal"
	"github.com/DreamCats/bcindex/internal/config"
)

// main 启动 bcindex 命令行工具，解析参数并执行对应子命令。
// 若参数无效或缺少子命令则打印用法并退出。
func main() {
	if len(os.Args) < 2 {
		internal.PrintUsage()
		os.Exit(1)
	}

	// Parse global flags and find subcommand
	configPath := ""
	repoPath := ""
	args := os.Args[1:]

	// Handle special flags that don't require subcommand
	for _, arg := range args {
		if arg == "-h" || arg == "-help" || arg == "--help" {
			internal.PrintUsage()
			os.Exit(0)
		}
		if arg == "-v" || arg == "-version" || arg == "--version" {
			fmt.Printf("bcindex version %s\n", internal.Version)
			os.Exit(0)
		}
	}

	// Find the subcommand (first non-flag argument that is a valid subcommand)
	validSubcommands := map[string]bool{
		"index":    true,
		"search":   true,
		"evidence": true,
		"stats":    true,
		"mcp":      true,
		"docgen":   true,
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
		internal.PrintUsage()
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
			internal.PrintUsage()
			os.Exit(0)
		} else if flag == "-v" || flag == "-version" || flag == "--version" {
			fmt.Printf("bcindex version %s\n", internal.Version)
			os.Exit(0)
		} else if strings.HasPrefix(flag, "-") {
			fmt.Fprintf(os.Stderr, "Error: Unknown global flag: %s\n\n", flag)
			internal.PrintUsage()
			os.Exit(1)
		}
	}

	// Load configuration
	cfg, err := internal.LoadConfig(configPath)
	if err != nil {
		if config.IsConfigNotFound(err) {
			if subcommand := args[subcommandIndex]; subcommand == "index" {
				if notFoundErr, ok := err.(*config.ConfigNotFoundError); ok {
					created, createErr := config.WriteDefaultTemplate(notFoundErr.RequestedPath)
					if createErr != nil {
						fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
						fmt.Fprintf(os.Stderr, "Also failed to create default config at %s: %v\n\n", notFoundErr.RequestedPath, createErr)
						internal.PrintConfigExample()
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
			internal.PrintConfigExample()
			os.Exit(1)
		}
		log.Fatalf("Failed to load config: %v\n", err)
	}

	// Override repo path if specified
	if repoPath != "" {
		cfg.Repo.Path = repoPath
	}

	repoRoot, err := internal.ResolveRepoRoot(cfg.Repo.Path)
	if err != nil {
		log.Fatalf("Failed to resolve repository root: %v\n", err)
	}
	cfg.Repo.Path = repoRoot

	dbPath, err := internal.DefaultDBPath(repoRoot)
	if err != nil {
		log.Fatalf("Failed to determine database path: %v\n", err)
	}
	cfg.Database.Path = dbPath

	// Execute subcommand
	subcommand := args[subcommandIndex]
	subcommandArgs := args[subcommandIndex+1:]

	if subcommand != "evidence" {
		if err := internal.SetupLogging(subcommand, repoRoot); err != nil {
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
	case "mcp":
		handleMCP(cfg, repoRoot, subcommandArgs)
	case "docgen":
		handleDocGen(cfg, repoRoot, subcommandArgs)
	default:
		fmt.Printf("Unknown subcommand: %s\n\n", subcommand)
		internal.PrintUsage()
		os.Exit(1)
	}
}
