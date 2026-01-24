package mcpserver

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DreamCats/bcindex/internal/config"
)

// resolveRepoRoot resolves the repository root path.
// It mirrors the CLI behavior by using the git top-level (if present).
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

// gitTopLevel returns the git repository root directory path.
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

func prepareConfig(base *config.Config, repoPath string) (*config.Config, error) {
	repoRoot, err := resolveRepoRoot(repoPath)
	if err != nil {
		return nil, err
	}
	selectedRoot, dbPath, err := selectRepoRootAndDB(repoRoot)
	if err != nil {
		return nil, err
	}
	cfg := *base
	cfg.Repo.Path = selectedRoot
	cfg.Database.Path = dbPath
	return &cfg, nil
}

func selectRepoRootAndDB(repoRoot string) (string, string, error) {
	candidates := repoRootCandidates(repoRoot)
	for _, candidate := range candidates {
		dbPath, err := defaultDBPath(candidate)
		if err != nil {
			return "", "", err
		}
		ok, err := hasIndexedRepo(dbPath, candidate)
		if err != nil {
			continue
		}
		if ok {
			return candidate, dbPath, nil
		}
	}
	dbPath, err := defaultDBPath(repoRoot)
	if err != nil {
		return "", "", err
	}
	return repoRoot, dbPath, nil
}

func repoRootCandidates(currentRoot string) []string {
	candidates := []string{currentRoot}
	worktrees, err := gitWorktrees(currentRoot)
	if err != nil || len(worktrees) == 0 {
		return candidates
	}
	primary := worktrees[0]
	if primary != "" && primary != currentRoot {
		candidates = append(candidates, primary)
	}
	for _, wt := range worktrees[1:] {
		if wt == "" || wt == currentRoot || wt == primary {
			continue
		}
		candidates = append(candidates, wt)
	}
	return candidates
}

func gitWorktrees(dir string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	var roots []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if path == "" {
			continue
		}
		roots = append(roots, normalizePath(path))
	}
	return roots, nil
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path
}

func hasIndexedRepo(dbPath string, repoRoot string) (bool, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}

	dsn := readOnlySQLiteDSN(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var exists int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='repositories'",
	).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM repositories WHERE root_path = ? AND symbol_count > 0",
		repoRoot,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func readOnlySQLiteDSN(path string) string {
	path = filepath.ToSlash(path)
	u := url.URL{Scheme: "file", Path: path}
	values := url.Values{}
	values.Set("mode", "ro")
	u.RawQuery = values.Encode()
	return u.String()
}
