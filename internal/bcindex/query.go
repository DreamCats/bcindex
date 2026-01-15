package bcindex

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blevesearch/bleve/v2"
)

func QueryRepo(paths RepoPaths, meta *RepoMeta, query string, qtype string, topK int) ([]SearchHit, error) {
	if err := ensureIndex(paths, qtype); err != nil {
		return nil, err
	}
	switch qtype {
	case "text":
		return queryText(paths, meta, query, topK)
	case "symbol":
		return querySymbols(paths, query, topK)
	case "mixed", "":
		return queryMixed(paths, meta, query, topK)
	default:
		return nil, fmt.Errorf("unknown query type: %s", qtype)
	}
}

func queryText(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	index, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return nil, err
	}
	defer index.Close()

	hits, err := searchText(index, meta.Root, query, topK)
	if err != nil {
		return nil, err
	}
	return hits, nil
}

func querySymbols(paths RepoPaths, query string, topK int) ([]SearchHit, error) {
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	symbols, err := store.SearchSymbols(query, topK)
	if err != nil {
		return nil, err
	}

	var hits []SearchHit
	for _, sym := range symbols {
		snippet := sym.Doc
		if snippet == "" {
			snippet = strings.TrimSpace(strings.Join([]string{sym.Pkg, sym.Recv}, " "))
		}
		hits = append(hits, SearchHit{
			Kind:    "symbol",
			Name:    sym.Name,
			File:    sym.File,
			Line:    sym.Line,
			Score:   1.0,
			Snippet: snippet,
		})
	}
	return hits, nil
}

func queryMixed(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	textHits, err := queryText(paths, meta, query, topK)
	if err != nil {
		return nil, err
	}
	symbolHits, err := querySymbols(paths, query, topK)
	if err != nil {
		return nil, err
	}

	hitMap := make(map[string]SearchHit)
	for _, hit := range symbolHits {
		key := fmt.Sprintf("s:%s:%d:%s", hit.File, hit.Line, hit.Name)
		hit.Score = 1.0
		hitMap[key] = hit
	}
	for _, hit := range textHits {
		key := fmt.Sprintf("t:%s:%d", hit.File, hit.Line)
		if existing, ok := hitMap[key]; ok {
			existing.Score += 0.5
			if existing.Snippet == "" {
				existing.Snippet = hit.Snippet
			}
			hitMap[key] = existing
			continue
		}
		hit.Score = 0.6
		hitMap[key] = hit
	}

	var merged []SearchHit
	for _, hit := range hitMap {
		merged = append(merged, hit)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score == merged[j].Score {
			return merged[i].File < merged[j].File
		}
		return merged[i].Score > merged[j].Score
	})
	if len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}

func searchText(index bleve.Index, root string, query string, topK int) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 10
	}

	contentQuery := bleve.NewMatchQuery(query)
	contentQuery.SetField("content")
	pathQuery := bleve.NewMatchQuery(query)
	pathQuery.SetField("path")
	titleQuery := bleve.NewMatchQuery(query)
	titleQuery.SetField("title")

	disjunction := bleve.NewDisjunctionQuery(contentQuery, pathQuery, titleQuery)

	req := bleve.NewSearchRequestOptions(disjunction, topK, 0, false)
	req.Fields = []string{"path", "kind", "title", "line_start", "line_end"}

	res, err := index.Search(req)
	if err != nil {
		return nil, err
	}

	var hits []SearchHit
	for _, hit := range res.Hits {
		pathVal, _ := hit.Fields["path"].(string)
		lineStart := parseLineField(hit.Fields["line_start"])
		snippet := ""
		if lineStart > 0 {
			snippet = readLine(root, pathVal, lineStart)
		}
		if snippet == "" {
			lineStart, snippet = findMatchLine(root, pathVal, query)
		}
		hits = append(hits, SearchHit{
			Kind:    "text",
			File:    pathVal,
			Line:    lineStart,
			Score:   hit.Score,
			Snippet: snippet,
		})
	}
	return hits, nil
}

func parseLineField(val any) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func readLine(root, rel string, target int) string {
	if target <= 0 {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 1
	for scanner.Scan() {
		if line == target {
			return strings.TrimSpace(scanner.Text())
		}
		line++
	}
	return ""
}

func findMatchLine(root, rel, query string) (int, string) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 1
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, query) {
			return line, strings.TrimSpace(text)
		}
		line++
	}
	return 0, ""
}
