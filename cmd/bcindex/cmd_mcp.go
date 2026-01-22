package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DreamCats/bcindex/cmd/bcindex/internal"
	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/mcpserver"
)

// handleMCP implements the MCP stdio server subcommand
func handleMCP(cfg *config.Config, repoRoot string, args []string) {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `USAGE:
    bcindex mcp

DESCRIPTION:
    Run an MCP stdio server exposing:
      - bcindex_locate
      - bcindex_context
      - bcindex_refs
`)
	}

	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse arguments: %v", err)
	}

	server := mcpserver.New(cfg, repoRoot, internal.Version)
	if err := server.Run(context.Background()); err != nil {
		log.Fatalf("MCP server failed: %v", err)
	}
}
