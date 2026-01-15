package bcindex

import (
	"bufio"
	"context"
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
	case "vector":
		hits, _, err := queryVector(paths, meta, query, topK, true)
		return hits, err
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

	if err := store.InitSchema(false); err != nil {
		return nil, err
	}

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
			Source:  "symbol",
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
	vectorCfg, vectorEnabled, err := loadVectorConfigForQuery()
	if err != nil {
		return nil, err
	}
	candidateTop := topK
	if vectorEnabled && vectorCfg.VectorRerankTop > candidateTop {
		candidateTop = vectorCfg.VectorRerankTop
	}

	textHits, err := queryText(paths, meta, query, candidateTop)
	if err != nil {
		return nil, err
	}
	symbolHits, err := querySymbols(paths, query, candidateTop)
	if err != nil {
		return nil, err
	}
	vectorHits, err := queryVectorCandidates(paths, meta, query, topK, vectorCfg, vectorEnabled, textHits, symbolHits)
	if err != nil {
		vectorHits = nil
	}

	type rankedHit struct {
		hit      SearchHit
		text     *SearchHit
		vector   *SearchHit
		symbol   *SearchHit
		priority int
	}
	hitMap := make(map[string]rankedHit)
	for _, hit := range symbolHits {
		key := fmt.Sprintf("%s:%d", hit.File, hit.Line)
		hit.Score = 1.0
		h := hit
		hitMap[key] = rankedHit{hit: h, symbol: &h, priority: 3}
	}
	for _, hit := range textHits {
		key := fmt.Sprintf("%s:%d", hit.File, hit.Line)
		if existing, ok := hitMap[key]; ok {
			if existing.priority == 3 {
				if existing.hit.Snippet == "" {
					existing.hit.Snippet = hit.Snippet
				}
				h := hit
				existing.text = &h
				hitMap[key] = existing
				continue
			}
			if hit.Score > existing.hit.Score {
				h := hit
				existing.hit = h
				existing.text = &h
				existing.priority = 2
				hitMap[key] = existing
			}
			continue
		}
		h := hit
		hitMap[key] = rankedHit{hit: h, text: &h, priority: 2}
	}
	for _, hit := range vectorHits {
		key := fmt.Sprintf("%s:%d", hit.File, hit.Line)
		if existing, ok := hitMap[key]; ok {
			h := hit
			existing.vector = &h
			if existing.hit.Snippet == "" {
				existing.hit.Snippet = hit.Snippet
			}
			hitMap[key] = existing
			continue
		}
		h := hit
		hitMap[key] = rankedHit{hit: h, vector: &h, priority: 1}
	}

	var merged []rankedHit
	for _, hit := range hitMap {
		vectorScore := 0.0
		textScore := 0.0
		symbolBoost := 0.0
		if hit.vector != nil {
			vectorScore = hit.vector.Score
		}
		if hit.text != nil {
			textScore = hit.text.Score
		}
		if hit.symbol != nil {
			symbolBoost = 1.0
		}
		hit.hit.Score = 0.5*vectorScore + 0.3*textScore + 0.2*symbolBoost
		hit.hit.Source = joinSources(hit.symbol != nil, hit.text != nil, hit.vector != nil)
		merged = append(merged, hit)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].priority != merged[j].priority {
			return merged[i].priority > merged[j].priority
		}
		if merged[i].hit.Score == merged[j].hit.Score {
			if merged[i].hit.File == merged[j].hit.File {
				return merged[i].hit.Line < merged[j].hit.Line
			}
			return merged[i].hit.File < merged[j].hit.File
		}
		return merged[i].hit.Score > merged[j].hit.Score
	})
	if len(merged) > topK {
		merged = merged[:topK]
	}
	results := make([]SearchHit, 0, len(merged))
	for _, hit := range merged {
		results = append(results, hit.hit)
	}
	return results, nil
}

func queryVectorCandidates(paths RepoPaths, meta *RepoMeta, query string, topK int, cfg VectorConfig, enabled bool, textHits []SearchHit, symbolHits []SearchHit) ([]SearchHit, error) {
	if !enabled {
		return nil, nil
	}
	if cfg.VectorRerankTop <= 0 {
		hits, _, err := queryVector(paths, meta, query, topK, false)
		return hits, err
	}
	candidates := make([]VectorCandidate, 0, len(textHits)+len(symbolHits))
	for _, hit := range textHits {
		candidates = append(candidates, VectorCandidate{File: hit.File, Line: hit.Line})
	}
	for _, hit := range symbolHits {
		candidates = append(candidates, VectorCandidate{File: hit.File, Line: hit.Line})
	}
	runtime, err := NewVectorRuntime(cfg)
	if err != nil {
		return nil, err
	}
	defer runtime.Close()

	ctx := context.Background()
	if err := runtime.EnsureCollection(ctx); err != nil {
		return nil, err
	}
	embeddings, err := runtime.embedder.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("vector embedding empty")
	}

	switch store := runtime.store.(type) {
	case *LocalVectorStore:
		hits, err := store.SearchSimilarCandidates(ctx, meta.RepoID, embeddings[0].Vector, candidates, topK)
		if err != nil {
			return nil, err
		}
		return vectorSearchResultsToHits(meta, hits), nil
	default:
		// fallback to full vector search when store doesn't support candidate search
		hits, err := runtime.store.SearchSimilar(ctx, cfg.QdrantCollection, meta.RepoID, embeddings[0].Vector, topK)
		if err != nil {
			return nil, err
		}
		return vectorSearchResultsToHits(meta, hits), nil
	}
}

func queryVector(paths RepoPaths, meta *RepoMeta, query string, topK int, required bool) ([]SearchHit, bool, error) {
	cfg, ok, err := LoadVectorConfigOptional()
	if err != nil {
		return nil, false, err
	}
	if !ok || !cfg.VectorEnabled {
		if required {
			return nil, false, fmt.Errorf("vector search requires enabled vector config")
		}
		return nil, false, nil
	}
	if strings.TrimSpace(cfg.VolcesAPIKey) == "" || strings.TrimSpace(cfg.VolcesModel) == "" {
		if required {
			return nil, false, fmt.Errorf("vector search requires volces_api_key and volces_model")
		}
		return nil, false, nil
	}
	runtime, err := NewVectorRuntime(cfg)
	if err != nil {
		return nil, true, err
	}
	defer runtime.Close()

	ctx := context.Background()
	if err := runtime.EnsureCollection(ctx); err != nil {
		return nil, true, err
	}
	embeddings, err := runtime.embedder.EmbedTexts(ctx, []string{query})
	if err != nil {
		return nil, true, err
	}
	if len(embeddings) == 0 {
		return nil, true, fmt.Errorf("vector embedding empty")
	}
	hits, err := runtime.store.SearchSimilar(ctx, cfg.QdrantCollection, meta.RepoID, embeddings[0].Vector, topK)
	if err != nil {
		return nil, true, err
	}
	return vectorSearchResultsToHits(meta, hits), true, nil
}

func searchText(index bleve.Index, root string, query string, topK int) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 10
	}

	contentQuery := bleve.NewMatchQuery(query)
	contentQuery.SetField("content")
	contentQuery.SetBoost(1.0)
	pathQuery := bleve.NewMatchQuery(query)
	pathQuery.SetField("path")
	pathQuery.SetBoost(1.5)
	titleQuery := bleve.NewMatchQuery(query)
	titleQuery.SetField("title")
	titleQuery.SetBoost(2.0)

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
			Source:  "text",
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

func joinSources(hasSymbol, hasText, hasVector bool) string {
	parts := make([]string, 0, 3)
	if hasSymbol {
		parts = append(parts, "symbol")
	}
	if hasText {
		parts = append(parts, "text")
	}
	if hasVector {
		parts = append(parts, "vector")
	}
	return strings.Join(parts, "+")
}

func loadVectorConfigForQuery() (VectorConfig, bool, error) {
	cfg, ok, err := LoadVectorConfigOptional()
	if err != nil {
		return VectorConfig{}, false, err
	}
	if !ok || !cfg.VectorEnabled {
		return cfg, false, nil
	}
	if strings.TrimSpace(cfg.VolcesAPIKey) == "" || strings.TrimSpace(cfg.VolcesModel) == "" {
		return cfg, false, nil
	}
	return cfg, true, nil
}

func vectorSearchResultsToHits(meta *RepoMeta, hits []VectorSearchResult) []SearchHit {
	results := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		line := hit.LineStart
		if line <= 0 {
			line = 1
		}
		snippet := readLine(meta.Root, hit.File, line)
		if snippet == "" {
			snippet = strings.TrimSpace(hit.Title)
		}
		results = append(results, SearchHit{
			Kind:    "vector",
			Source:  "vector",
			Name:    hit.Name,
			File:    hit.File,
			Line:    line,
			Score:   hit.Score,
			Snippet: snippet,
		})
	}
	return results
}
