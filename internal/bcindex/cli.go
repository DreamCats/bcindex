package bcindex

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Run(args []string) int {
	if len(args) < 2 {
		printUsage()
		return 1
	}
	switch args[1] {
	case "init":
		return runInit(args[2:])
	case "index":
		return runIndex(args[2:])
	case "query":
		return runQuery(args[2:])
	case "status":
		return runStatus(args[2:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[1])
		printUsage()
		return 1
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	root := fs.String("root", "", "repo root path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	resolved, err := resolveRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	paths, meta, err := InitRepo(resolved)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("repo_id: %s\nroot: %s\nmeta: %s\n", paths.RepoID, meta.Root, paths.MetaFile)
	return 0
}

func runIndex(args []string) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	root := fs.String("root", "", "repo root path")
	full := fs.Bool("full", true, "full index (only mode in MVP)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !*full {
		fmt.Fprintln(os.Stderr, "only full index is supported in MVP")
		return 1
	}
	resolved, err := resolveRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := IndexRepo(resolved); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("index completed")
	return 0
}

func runQuery(args []string) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	repo := fs.String("repo", "", "repo id or path")
	root := fs.String("root", "", "repo root path (overrides --repo)")
	qtype := fs.String("type", "mixed", "query type: text|symbol|mixed")
	query := fs.String("q", "", "query text")
	topK := fs.Int("top", 10, "max results")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "query text is required")
		return 1
	}

	cwd, _ := os.Getwd()
	paths, meta, err := ResolveRepo(*repo, *root, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	hits, err := QueryRepo(paths, meta, *query, strings.ToLower(*qtype), *topK)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printHits(hits)
	return 0
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	repo := fs.String("repo", "", "repo id or path")
	root := fs.String("root", "", "repo root path (overrides --repo)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cwd, _ := os.Getwd()
	paths, meta, err := ResolveRepo(*repo, *root, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	status, err := RepoStatus(paths, meta)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("repo_id: %s\n", status.RepoID)
	fmt.Printf("root: %s\n", status.Root)
	fmt.Printf("last_index_at: %s\n", status.LastIndexAt.Format(timeLayout()))
	fmt.Printf("symbols: %d\n", status.Symbols)
	fmt.Printf("text_docs: %d\n", status.TextDocs)
	fmt.Printf("index_dir: %s\n", filepath.Dir(paths.TextDir))
	return 0
}

func resolveRoot(root string) (string, error) {
	if strings.TrimSpace(root) != "" {
		return filepath.Abs(root)
	}
	cwd, _ := os.Getwd()
	return findGitRoot(cwd)
}

func printHits(hits []SearchHit) {
	if len(hits) == 0 {
		fmt.Println("no results")
		return
	}
	for _, hit := range hits {
		name := hit.Name
		if name == "" {
			name = "-"
		}
		line := hit.Line
		if line <= 0 {
			line = 1
		}
		snippet := strings.TrimSpace(hit.Snippet)
		fmt.Printf("%s\t%s\t%s:%d\t%.2f\t%s\n", hit.Kind, name, hit.File, line, hit.Score, snippet)
	}
}

func printUsage() {
	fmt.Println(`bcindex <command> [options]

Commands:
  init   --root <repo>
  index  --root <repo> --full
  query  --repo <id|path> --q <text> --type <text|symbol|mixed>
  status --repo <id|path>
`)
}

func timeLayout() string {
	return "2006-01-02 15:04:05"
}
