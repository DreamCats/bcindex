package bcindex

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var fileQueryPattern = regexp.MustCompile(`(?i)[\\w./-]+\\.(go|md|markdown|txt|json|yaml|yml|toml|sql|proto|ts|js|py|java|rs|c|cpp|h|hpp)`)

func querySearch(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	if err := ensureIndex(paths, "symbol"); err != nil {
		return nil, err
	}
	if fileToken := extractFileQuery(query); fileToken != "" {
		fileHits, err := searchFileHits(paths, fileToken, topK)
		if err == nil && len(fileHits) > 0 {
			return fileHits, nil
		}
	}
	hits, err := QueryRepo(paths, meta, query, "mixed", topK)
	if err != nil {
		return nil, err
	}
	return compactSearchHits(hits), nil
}

func extractFileQuery(query string) string {
	match := fileQueryPattern.FindString(query)
	if match == "" {
		return ""
	}
	return strings.Trim(match, "`'\"")
}

func searchFileHits(paths RepoPaths, token string, topK int) ([]SearchHit, error) {
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return nil, err
	}

	limit := topK
	if limit <= 0 {
		limit = 5
	}
	files, err := store.SearchFilesByName(token, limit)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	hits := make([]SearchHit, 0, len(files))
	for _, rel := range files {
		pkg, line := readGoPackageLine(paths.Root, rel)
		name := "-"
		snippet := ""
		if pkg != "" {
			name = pkg
			snippet = "package " + pkg
		} else {
			line = 1
			snippet = strings.TrimSpace(readLinesRange(paths.Root, rel, 1, 1, 2))
		}
		hits = append(hits, SearchHit{
			Kind:    "file",
			Source:  "file",
			Name:    name,
			File:    rel,
			Line:    line,
			Score:   1.0,
			Snippet: snippet,
		})
	}
	return hits, nil
}

func readGoPackageLine(root, rel string) (string, int) {
	if !strings.HasSuffix(strings.ToLower(rel), ".go") {
		return "", 0
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return "", 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		if line > 200 {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(text, "package") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) < 2 {
			continue
		}
		return fields[1], line
	}
	return "", 0
}

func compactSearchHits(hits []SearchHit) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	for i := range hits {
		hits[i].Relations = nil
		hits[i].DocLinks = nil
		hits[i].Snippet = compactSnippet(hits[i].Snippet, 1, 200)
	}
	return hits
}

func compactSnippet(snippet string, maxLines, maxChars int) string {
	if snippet == "" {
		return ""
	}
	lines := strings.Split(snippet, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	out := strings.TrimSpace(strings.Join(lines, "\n"))
	if maxChars > 0 && countRunes(out) > maxChars {
		runes := []rune(out)
		out = string(runes[:maxChars]) + "..."
	}
	return out
}
