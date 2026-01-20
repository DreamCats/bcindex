package mcpserver

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DreamCats/bcindex/internal/config"
)

// resolveRepoRoot resolves the repository root path.
// For git repositories with worktrees, it uses the git common directory
// to ensure all worktrees share the same index database.
func resolveRepoRoot(repoPath string) (string, error) {
	root := repoPath
	if root == "" || root == "." {
		root = "."
	}

	absPath, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	// Try git common-dir first (supports worktrees)
	// This returns the shared .git directory for all worktrees
	if gitCommonDir := gitCommonDir(absPath); gitCommonDir != "" {
		absPath = gitCommonDir
	} else if gitRoot := gitTopLevel(absPath); gitRoot != "" {
		// Fallback to git toplevel for non-worktree repos
		absPath = gitRoot
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}

	return absPath, nil
}

// gitCommonDir returns the git common directory path.
// For worktrees, this returns the shared .git directory.
// For regular repos, this returns the .git directory path.
func gitCommonDir(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(output))
	// Check if the command succeeded (some git versions return the flag name on error)
	if commonDir == "" || commonDir == "--git-common-dir" {
		return ""
	}
	return commonDir
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
	cfg := *base
	cfg.Repo.Path = repoRoot
	dbPath, err := defaultDBPath(repoRoot)
	if err != nil {
		return nil, err
	}
	cfg.Database.Path = dbPath
	return &cfg, nil
}
