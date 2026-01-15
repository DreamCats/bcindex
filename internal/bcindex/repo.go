package bcindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func InitRepo(root string) (RepoPaths, *RepoMeta, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return RepoPaths{}, nil, fmt.Errorf("abs root: %w", err)
	}
	paths, err := repoPaths(absRoot)
	if err != nil {
		return RepoPaths{}, nil, err
	}
	if err := ensureDir(paths.TextDir); err != nil {
		return RepoPaths{}, nil, err
	}
	if err := ensureDir(paths.SymbolDir); err != nil {
		return RepoPaths{}, nil, err
	}
	if err := ensureDir(paths.MetaDir); err != nil {
		return RepoPaths{}, nil, err
	}

	meta, err := LoadRepoMeta(paths)
	now := time.Now()
	if err != nil {
		if !os.IsNotExist(err) {
			return RepoPaths{}, nil, err
		}
		meta = &RepoMeta{
			RepoID:    paths.RepoID,
			Root:      absRoot,
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		meta.UpdatedAt = now
		meta.Root = absRoot
	}
	if err := SaveRepoMeta(paths, meta); err != nil {
		return RepoPaths{}, nil, err
	}
	return paths, meta, nil
}

func LoadRepoMeta(paths RepoPaths) (*RepoMeta, error) {
	data, err := os.ReadFile(paths.MetaFile)
	if err != nil {
		return nil, err
	}
	var meta RepoMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parse repo meta: %w", err)
	}
	return &meta, nil
}

func SaveRepoMeta(paths RepoPaths, meta *RepoMeta) error {
	if err := ensureDir(paths.MetaDir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repo meta: %w", err)
	}
	if err := os.WriteFile(paths.MetaFile, data, 0o644); err != nil {
		return fmt.Errorf("write repo meta: %w", err)
	}
	return nil
}

func ResolveRepo(repoArg, rootArg, cwd string) (RepoPaths, *RepoMeta, error) {
	if rootArg != "" {
		absRoot, err := filepath.Abs(rootArg)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		paths, err := repoPaths(absRoot)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		meta, err := loadRepoMetaOrHint(paths, absRoot)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		return applyMetaRoot(paths, meta), meta, nil
	}

	if repoArg == "" {
		root, err := findGitRoot(cwd)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		paths, err := repoPaths(root)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		meta, err := loadRepoMetaOrHint(paths, root)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		return applyMetaRoot(paths, meta), meta, nil
	}

	if st, err := os.Stat(repoArg); err == nil && st.IsDir() {
		absRoot, err := filepath.Abs(repoArg)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		paths, err := repoPaths(absRoot)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		meta, err := loadRepoMetaOrHint(paths, absRoot)
		if err != nil {
			return RepoPaths{}, nil, err
		}
		return applyMetaRoot(paths, meta), meta, nil
	}

	paths, err := repoPathsFromID(repoArg)
	if err != nil {
		return RepoPaths{}, nil, err
	}
	meta, err := loadRepoMetaOrHint(paths, "")
	if err != nil {
		return RepoPaths{}, nil, err
	}
	return applyMetaRoot(paths, meta), meta, nil
}

func loadRepoMetaOrHint(paths RepoPaths, root string) (*RepoMeta, error) {
	meta, err := LoadRepoMeta(paths)
	if err == nil {
		return meta, nil
	}
	if os.IsNotExist(err) {
		if root == "" {
			root = paths.Root
		}
		if root != "" {
			return nil, fmt.Errorf("repo not initialized. run: bcindex index --root %s --full", root)
		}
		return nil, fmt.Errorf("repo not initialized. missing %s", paths.MetaFile)
	}
	return nil, err
}

func applyMetaRoot(paths RepoPaths, meta *RepoMeta) RepoPaths {
	if paths.Root == "" {
		paths.Root = meta.Root
	}
	return paths
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("ensure dir %s: %w", path, err)
	}
	return nil
}
