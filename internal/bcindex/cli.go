package bcindex

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	case "version":
		return runVersion(args[2:])
	case "config":
		return runConfig(args[2:])
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
		paths, _, err := InitRepo(resolved)
		if err == nil {
			if !textIndexExists(paths) || !symbolIndexExists(paths) {
				fmt.Fprintln(os.Stderr, "index missing; running full index first")
				if err := IndexRepoWithProgress(resolved, reporter); err != nil {
					if warn := (*IndexWarning)(nil); errors.As(err, &warn) {
						fmt.Fprintln(os.Stderr, warn.Error())
					} else {
						fmt.Fprintln(os.Stderr, err)
						return 1
					}
				}
				fmt.Println(indexCompletionSummary(resolved, false))
				return 0
			}
		}
		if err := IndexRepoDeltaFromGit(resolved, *diff, reporter); err != nil {
			if warn := (*IndexWarning)(nil); errors.As(err, &warn) {
				fmt.Fprintln(os.Stderr, warn.Error())
			} else {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		fmt.Println(indexCompletionSummary(resolved, true))
		return 0
	}
	if err := IndexRepoWithProgress(resolved, reporter); err != nil {
		if warn := (*IndexWarning)(nil); errors.As(err, &warn) {
			fmt.Fprintln(os.Stderr, warn.Error())
		} else {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Println(indexCompletionSummary(resolved, false))
	return 0
}

func runWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	root := fs.String("root", "", "repo root path")
	interval := fs.Duration("interval", 3*time.Second, "poll interval")
	debounceInterval := fs.Duration("debounce", 2*time.Second, "debounce duration")
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
	debounce := *debounceInterval
	if debounce < *interval {
		debounce = *interval
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	var lastHash string
	var pending []FileChange
	var lastChange time.Time

	for range ticker.C {
		statusOut, changes, err := gitStatusSnapshot(resolved)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		hash := hashString(statusOut)
		if hash == "" && len(changes) == 0 {
			pending = nil
			lastChange = time.Time{}
			lastHash = hash
			continue
		}
		if hash != lastHash {
			pending = changes
			lastChange = time.Now()
			lastHash = hash
		}
		if len(pending) == 0 || lastChange.IsZero() {
			continue
		}
		if time.Since(lastChange) < debounce {
			continue
		}
		reporter := NewIndexProgress(*progress)
		if err := IndexRepoDelta(resolved, pending, reporter); err != nil {
			if warn := (*IndexWarning)(nil); errors.As(err, &warn) {
				fmt.Fprintln(os.Stderr, warn.Error())
			} else {
				fmt.Fprintln(os.Stderr, err)
			}
		}
		pending = nil
	}
	return 0
}

func indexCompletionSummary(root string, isDiff bool) string {
	phase := "text+symbol"
	vectorEnabled := false
	if cfg, ok, err := LoadVectorConfigOptional(); err == nil && ok {
		vectorEnabled = cfg.VectorEnabled
	}
	if vectorEnabled {
		phase = "text+symbol+vector"
	}
	if isDiff {
		return fmt.Sprintf("index completed (diff: %s)", phase)
	}
	return fmt.Sprintf("index completed (%s)", phase)
}

func runQuery(args []string) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	repo := fs.String("repo", "", "repo id or path")
	root := fs.String("root", "", "repo root path (overrides --repo)")
	qtype := fs.String("type", "mixed", "query type: text|symbol|mixed|vector")
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

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	root := fs.String("root", "", "repo root path (optional)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	var candidates []string
	if strings.TrimSpace(*root) != "" {
		if abs, err := filepath.Abs(*root); err == nil {
			candidates = append(candidates, abs)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
		if gitRoot, err := findGitRoot(cwd); err == nil {
			candidates = append(candidates, gitRoot)
		}
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	seen := make(map[string]struct{})
	for _, base := range candidates {
		if base == "" {
			continue
		}
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		version, err := ReadVersion(base)
		if err == nil && version != "" {
			fmt.Printf("version: %s\n", version)
			return 0
		}
	}
	fmt.Println("version: unknown")
	return 0
}

func runConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "config requires a subcommand: init")
		return 1
	}
	switch args[0] {
	case "init":
		return runConfigInit(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return 1
	}
}

func runConfigInit(args []string) int {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing config")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	path, err := vectorConfigPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *force {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	created, err := WriteDefaultVectorConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("config: %s\n", created)
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
  watch  --root <repo> [--interval 3s] [--debounce 2s] [--progress]
  query  --repo <id|path> --q <text> --type <text|symbol|mixed|vector> [--json] [--progress]
  status --repo <id|path>
  version [--root <repo>]
  config init [--force]
`)
}

func hashString(input string) string {
	if input == "" {
		return ""
	}
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

func timeLayout() string {
	return "2006-01-02 15:04:05"
}
