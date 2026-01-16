package bcindex

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type IgnoreRule struct {
	Pattern  string
	IsDir    bool
	Negated  bool
	MatchFn  func(string) bool
}

type IgnoreMatcher struct {
	rules   []IgnoreRule
	rootDir string
}

func NewIgnoreMatcher(rootDir string) *IgnoreMatcher {
	return &IgnoreMatcher{
		rootDir: rootDir,
		rules:   make([]IgnoreRule, 0),
	}
}

func (m *IgnoreMatcher) LoadGitignore(gitignorePath string) error {
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return m.ParseGitignore(content)
}

func (m *IgnoreMatcher) ParseGitignore(content []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule := IgnoreRule{
			Pattern: line,
		}

		if strings.HasPrefix(line, "!") {
			rule.Negated = true
			rule.Pattern = strings.TrimPrefix(line, "!")
		}

		if strings.HasSuffix(rule.Pattern, "/") {
			rule.IsDir = true
			rule.Pattern = strings.TrimSuffix(rule.Pattern, "/")
		}

		if strings.HasPrefix(rule.Pattern, "/") {
			rule.Pattern = strings.TrimPrefix(rule.Pattern, "/")
			rule.MatchFn = func(pattern string) func(string) bool {
				return func(path string) bool {
					matched, _ := doublestar.Match(pattern, path)
					return matched
				}
			}(rule.Pattern)
		} else {
			rule.MatchFn = func(pattern string) func(string) bool {
				return func(path string) bool {
					matched, _ := doublestar.Match("**/"+pattern, path)
					if !matched {
						matched, _ = doublestar.Match(pattern, path)
					}
					return matched
				}
			}(rule.Pattern)
		}

		m.rules = append(m.rules, rule)
	}

	return scanner.Err()
}

func (m *IgnoreMatcher) AddPattern(pattern string) {
	rule := IgnoreRule{
		Pattern: pattern,
	}

	if strings.HasPrefix(pattern, "!") {
		rule.Negated = true
		rule.Pattern = strings.TrimPrefix(pattern, "!")
	}

	if strings.HasSuffix(rule.Pattern, "/") {
		rule.IsDir = true
		rule.Pattern = strings.TrimSuffix(rule.Pattern, "/")
	}

	if strings.HasPrefix(rule.Pattern, "/") {
		rule.Pattern = strings.TrimPrefix(rule.Pattern, "/")
		rule.MatchFn = func(p string) func(string) bool {
			return func(path string) bool {
				matched, _ := doublestar.Match(p, path)
				return matched
			}
		}(rule.Pattern)
	} else {
		rule.MatchFn = func(p string) func(string) bool {
			return func(path string) bool {
				matched, _ := doublestar.Match("**/"+p, path)
				if !matched {
					matched, _ = doublestar.Match(p, path)
				}
				return matched
			}
		}(rule.Pattern)
	}

	m.rules = append(m.rules, rule)
}

func (m *IgnoreMatcher) Match(relPath string, isDir bool) bool {
	path := filepath.ToSlash(relPath)

	excluded := false
	for _, rule := range m.rules {
		if rule.IsDir && !isDir {
			continue
		}

		matches := rule.MatchFn(path)
		if matches {
			if rule.Negated {
				excluded = false
			} else {
				excluded = true
			}
		}
	}

	return excluded
}

func loadGitignorePatterns(root string) (*IgnoreMatcher, error) {
	matcher := NewIgnoreMatcher(root)

	gitignorePath := filepath.Join(root, ".gitignore")
	if err := matcher.LoadGitignore(gitignorePath); err != nil {
		return nil, err
	}

	return matcher, nil
}
