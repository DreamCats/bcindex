package bcindex

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

type FileFilter struct {
	cfg             IndexConfig
	gitignoreMatcher *IgnoreMatcher
	once            sync.Once
	loadErr         error
}

func NewFileFilter(cfg IndexConfig, root string) *FileFilter {
	return &FileFilter{
		cfg: cfg,
		gitignoreMatcher: NewIgnoreMatcher(root),
	}
}

func (f *FileFilter) loadGitignore(root string) error {
	if !f.cfg.UseGitignore {
		return nil
	}
	var err error
	f.once.Do(func() {
		f.gitignoreMatcher, err = loadGitignorePatterns(root)
	})
	return err
}

func (f *FileFilter) ShouldIndex(relPath string) bool {
	if f.cfg.UseGitignore && f.gitignoreMatcher != nil {
		isDir := strings.HasSuffix(relPath, "/") || filepath.Ext(relPath) == ""
		if f.gitignoreMatcher.Match(relPath, isDir) {
			return false
		}
	}

	for _, pattern := range f.cfg.Exclude {
		matched, _ := doublestar.Match(pattern, relPath)
		if matched {
			return false
		}
		base := filepath.Base(relPath)
		matched, _ = doublestar.Match(pattern, base)
		if matched {
			return false
		}
	}

	pathParts := strings.Split(filepath.ToSlash(relPath), "/")
	for i, part := range pathParts {
		for _, excludeDir := range f.cfg.ExcludeDirs {
			if part == excludeDir {
				return false
			}
		}
		if i < len(pathParts)-1 {
			for _, excludeDir := range f.cfg.ExcludeDirs {
				if strings.HasSuffix(excludeDir, "/") {
					dirPattern := strings.TrimSuffix(excludeDir, "/")
					if part == dirPattern {
						return false
					}
				}
			}
		}
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".go", ".md", ".markdown":
		return true
	default:
		return false
	}
}

var globalFilter *FileFilter
var globalFilterMu sync.Mutex

func InitFileFilter(cfg IndexConfig, root string) (*FileFilter, error) {
	filter := NewFileFilter(cfg, root)
	if err := filter.loadGitignore(root); err != nil {
		return nil, err
	}
	globalFilterMu.Lock()
	globalFilter = filter
	globalFilterMu.Unlock()
	return filter, nil
}

func GetGlobalFilter() *FileFilter {
	globalFilterMu.Lock()
	defer globalFilterMu.Unlock()
	return globalFilter
}
