package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveRepoRoot resolves the absolute path of the repository root directory.
// It first converts the relative path to an absolute path.
// If the path is a Git repository, it returns the Git root directory.
// Otherwise, it returns the absolute path.
func ResolveRepoRoot(repoPath string) (string, error) {
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

// gitTopLevel 返回给定目录所在 Git 仓库的根路径。
// 若未找到 .git 则返回空字符串。
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

// DefaultDBPath 基于仓库根目录生成默认的 BoltDB 数据库路径。
// 返回路径字符串或构造失败时的 error。
func DefaultDBPath(repoRoot string) (string, error) {
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

// sanitizeRepoName 将仓库名称中的危险字符替换为安全下划线。
// 用于生成文件系统友好的标识符。
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
