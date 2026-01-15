package bcindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	baseDirName  = ".bcindex"
	reposDirName = "repos"
)

func baseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, baseDirName), nil
}

func repoID(root string) string {
	sum := sha1.Sum([]byte(root))
	return hex.EncodeToString(sum[:])
}

func repoPaths(root string) (RepoPaths, error) {
	base, err := baseDir()
	if err != nil {
		return RepoPaths{}, err
	}
	id := repoID(root)
	repoDir := filepath.Join(base, reposDirName, id)
	metaDir := filepath.Join(repoDir, "meta")
	return RepoPaths{
		RepoID:    id,
		Root:      root,
		BaseDir:   base,
		RepoDir:   repoDir,
		TextDir:   filepath.Join(repoDir, "text"),
		SymbolDir: filepath.Join(repoDir, "symbol"),
		MetaDir:   metaDir,
		MetaFile:  filepath.Join(metaDir, "repo.json"),
	}, nil
}

func repoPathsFromID(id string) (RepoPaths, error) {
	base, err := baseDir()
	if err != nil {
		return RepoPaths{}, err
	}
	repoDir := filepath.Join(base, reposDirName, id)
	metaDir := filepath.Join(repoDir, "meta")
	return RepoPaths{
		RepoID:    id,
		BaseDir:   base,
		RepoDir:   repoDir,
		TextDir:   filepath.Join(repoDir, "text"),
		SymbolDir: filepath.Join(repoDir, "symbol"),
		MetaDir:   metaDir,
		MetaFile:  filepath.Join(metaDir, "repo.json"),
	}, nil
}

func symbolDBPath(paths RepoPaths) string {
	return filepath.Join(paths.SymbolDir, "symbols.db")
}
