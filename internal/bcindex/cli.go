package bcindex

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	case "watch":
		return runWatch(args[2:])
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
	full := fs.Bool("full", false, "full index")
	diff := fs.String("diff", "", "git diff revision for incremental index")
	progress := fs.Bool("progress", DefaultProgressEnabled(), "show progress")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *full && strings.TrimSpace(*diff) != "" {
		fmt.Fprintln(os.Stderr, "cannot use --full with --diff")
		return 1
	}
	resolved, err := resolveRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	reporter := NewIndexProgress(*progress)
	if strings.TrimSpace(*diff) != "" {
		if err := IndexRepoDeltaFromGit(resolved, *diff, reporter); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("index completed (diff)")
		return 0
	}
	if err := IndexRepoWithProgress(resolved, reporter); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("index completed")
	return 0
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	root := fs.String("root", "", "repo root path")
	interval := fs.Duration("interval", 3*time.Second, "poll interval")
	progress := fs.Bool("progress", DefaultProgressEnabled(), "show progress")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	resolved, err := resolveRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("watching %s (interval %s)\n", resolved, interval.String())
	for {
		changes, err := gitStatusChanges(resolved)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			time.Sleep(*interval)
			continue
		}
		if len(changes) > 0 {
			reporter := NewIndexProgress(*progress)
			if err := IndexRepoDelta(resolved, changes, reporter); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
		time.Sleep(*interval)
	}
}

func runQuery(args []string) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	repo := fs.String("repo", "", "repo id or path")
	root := fs.String("root", "", "repo root path (overrides --repo)")
	qtype := fs.String("type", "mixed", "query type: text|symbol|mixed")
	query := fs.String("q", "", "query text")
	topK := fs.Int("top", 10, "max results")
	jsonOut := fs.Bool("json", false, "output JSON")
	progress := fs.Bool("progress", DefaultProgressEnabled(), "show progress")
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

	stop := StartSpinner(*progress, "searching")
	hits, err := QueryRepo(paths, meta, *query, strings.ToLower(*qtype), *topK)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *jsonOut {
		if err := printHitsJSON(hits); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
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

func printHitsJSON(hits []SearchHit) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(hits)
}

func printUsage() {
	fmt.Println(`bcindex <command> [options]

Commands:
  init   --root <repo>
  index  --root <repo> [--full|--diff <rev>] [--progress]
  watch  --root <repo> [--interval 3s] [--progress]
  query  --repo <id|path> --q <text> --type <text|symbol|mixed> [--json] [--progress]
  status --repo <id|path>
`)
}

func timeLayout() string {
	return "2006-01-02 15:04:05"
}
