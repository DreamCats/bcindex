package internal

import (
	"fmt"
	"os"
	"strings"
)

const Version = "1.0.6"

// PrintUsage 向 stderr 输出 bcindex 的用法与可用子命令列表。
// 无返回值，直接退出程序。
func PrintUsage() {
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

    mcp
        Run MCP stdio server (tools: bcindex_search, bcindex_evidence)

    docgen
        Generate documentation for Go code using LLM

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

    # Run MCP server over stdio
    bcindex mcp

    # Generate documentation (dry run)
    bcindex docgen --dry-run

For detailed help on each command, use:
    bcindex <command> -help
`, Version)
}

// StringList is a flag.Value that collects multiple strings
type StringList []string

// String 返回 stringList 的逗号连接形式。
// 满足 fmt.Stringer 与 flag.Value 接口要求。
func (s *StringList) String() string {
	return strings.Join(*s, ",")
}

// Set 将单个字符串追加到 stringList 并返回错误（始终为 nil）。
// 该方法用于实现 flag.Value 接口，允许多次 -flag 传入。
func (s *StringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}
