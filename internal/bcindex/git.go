package bcindex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func findGitRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no git root found from %s", start)
}

func listTrackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return walkFiles(root)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
	}
	return files, nil
}

func walkFiles(root string) ([]string, error) {
	cfg, _, err := LoadIndexConfigOptional()
	if err != nil {
		cfg = defaultIndexConfig()
	}
	filter := NewFileFilter(cfg, root)
	filter.loadGitignore(root)

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !filter.ShouldIndex(rel + "/") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if filter.ShouldIndex(rel) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func shouldIndex(rel string) bool {
	filter := GetGlobalFilter()
	if filter != nil {
		result := filter.ShouldIndex(rel)
		if !result {
			LogDebug("File filtered by filter", map[string]interface{}{"file": rel})
		}
		return result
	}

	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".go", ".md", ".markdown":
		return true
	default:
		LogDebug("File filtered by extension", map[string]interface{}{
			"file": rel,
			"ext": ext,
		})
		return false
	}
}
