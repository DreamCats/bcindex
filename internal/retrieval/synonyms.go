package retrieval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type synonymsFile struct {
	Version  int                 `yaml:"version"`
	Synonyms map[string][]string `yaml:"synonyms"`
}

// SynonymsExpander provides query expansion based on repo-level synonym groups.
type SynonymsExpander struct {
	groups []synonymGroup
}

type synonymGroup struct {
	canonical string
	terms     []string
	normTerms []string
}

// SynonymMatch represents a matched synonym group.
type SynonymMatch struct {
	Canonical string
	Terms     []string
}

// LoadSynonymsForRepo loads the synonyms file with repo-root resolution.
func LoadSynonymsForRepo(repoRoot string, synonymsFile string) (*SynonymsExpander, error) {
	if strings.TrimSpace(synonymsFile) == "" {
		return nil, nil
	}
	path := synonymsFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return LoadSynonymsFile(path)
}

// LoadSynonymsFile loads a synonyms file if it exists.
func LoadSynonymsFile(path string) (*SynonymsExpander, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read synonyms file: %w", err)
	}

	var file synonymsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse synonyms file: %w", err)
	}

	return NewSynonymsExpander(file.Synonyms), nil
}

// NewSynonymsExpander builds a synonym expander from a map.
func NewSynonymsExpander(synonyms map[string][]string) *SynonymsExpander {
	if len(synonyms) == 0 {
		return nil
	}

	keys := make([]string, 0, len(synonyms))
	for k := range synonyms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	groups := make([]synonymGroup, 0, len(keys))
	for _, canonical := range keys {
		aliases := synonyms[canonical]
		terms, normTerms := buildTerms(canonical, aliases)
		if len(terms) == 0 {
			continue
		}
		groups = append(groups, synonymGroup{
			canonical: canonical,
			terms:     terms,
			normTerms: normTerms,
		})
	}

	if len(groups) == 0 {
		return nil
	}
	return &SynonymsExpander{groups: groups}
}

// Expand returns expanded query text for embedding and FTS query for keyword search.
func (e *SynonymsExpander) Expand(query string) (string, string, []SynonymMatch) {
	if e == nil {
		return query, query, nil
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return query, query, nil
	}

	normQuery := normalizeTerm(trimmed)
	if normQuery == "" {
		return query, query, nil
	}

	matches := make([]SynonymMatch, 0)
	for _, g := range e.groups {
		if matchesGroup(normQuery, g.normTerms) {
			matches = append(matches, SynonymMatch{
				Canonical: g.canonical,
				Terms:     g.terms,
			})
		}
	}

	if len(matches) == 0 {
		return query, query, nil
	}

	expandedTerms := uniqueTerms(matches)
	expandedQuery := strings.TrimSpace(trimmed + " " + strings.Join(expandedTerms, " "))
	ftsQuery := buildFTSQuery(matches)
	if ftsQuery == "" {
		ftsQuery = query
	}

	return expandedQuery, ftsQuery, matches
}

func matchesGroup(normQuery string, normTerms []string) bool {
	for _, term := range normTerms {
		if term == "" {
			continue
		}
		if strings.Contains(normQuery, term) {
			return true
		}
	}
	return false
}

func buildTerms(canonical string, aliases []string) ([]string, []string) {
	terms := make([]string, 0, 1+len(aliases))
	normTerms := make([]string, 0, 1+len(aliases))
	seen := make(map[string]bool)

	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		norm := normalizeTerm(term)
		if norm == "" || seen[norm] {
			return
		}
		terms = append(terms, term)
		normTerms = append(normTerms, norm)
		seen[norm] = true
	}

	add(canonical)
	for _, alias := range aliases {
		add(alias)
	}

	return terms, normTerms
}

func uniqueTerms(matches []SynonymMatch) []string {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, match := range matches {
		for _, term := range match.Terms {
			norm := normalizeTerm(term)
			if norm == "" || seen[norm] {
				continue
			}
			seen[norm] = true
			out = append(out, term)
		}
	}
	return out
}

func normalizeTerm(term string) string {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return ""
	}
	term = strings.ReplaceAll(term, "_", " ")
	term = strings.ReplaceAll(term, "-", " ")
	term = strings.Join(strings.Fields(term), " ")
	return term
}

func buildFTSQuery(matches []SynonymMatch) string {
	if len(matches) == 0 {
		return ""
	}

	groups := make([]string, 0, len(matches))
	for _, match := range matches {
		terms := make([]string, 0, len(match.Terms))
		for _, term := range match.Terms {
			quoted := quoteFTSTerm(term)
			if quoted != "" {
				terms = append(terms, quoted)
			}
		}
		if len(terms) == 0 {
			continue
		}
		groups = append(groups, "("+strings.Join(terms, " OR ")+")")
	}

	if len(groups) == 0 {
		return ""
	}
	if len(groups) == 1 {
		return groups[0]
	}
	return strings.Join(groups, " AND ")
}

func quoteFTSTerm(term string) string {
	term = strings.TrimSpace(term)
	if term == "" {
		return ""
	}
	if strings.ContainsAny(term, " \t\r\n-") {
		escaped := strings.ReplaceAll(term, `"`, `""`)
		return `"` + escaped + `"`
	}
	return term
}
