package bcindex

import (
	"fmt"
	"sort"
	"strings"
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

func querySymbols(paths RepoPaths, query string, topK int) ([]SearchHit, error) {
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
		limit = 10
	}
	variants := buildQueryVariants(query)
	if len(variants) == 0 {
		return nil, nil
	}
	candidateTop := expandCandidateLimit(limit, len(variants))
	weighted := make([]weightedHit, 0, candidateTop*len(variants))
	for _, variant := range variants {
		symbols, err := store.SearchSymbols(variant.Text, candidateTop)
		if err != nil {
			return nil, err
		}
		hits := symbolsToHits(symbols)
		for _, hit := range hits {
			weighted = append(weighted, weightedHit{
				Hit:    hit,
				Weight: variant.Weight,
			})
		}
	}
	return mergeWeightedHits(weighted, limit), nil
}

func querySymbolsExact(paths RepoPaths, query string, topK int) ([]SearchHit, error) {
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
		limit = 10
	}
	symbols, err := store.SearchSymbols(query, limit)
	if err != nil {
		return nil, err
	}
	return symbolsToHits(symbols), nil
}

func symbolsToHits(symbols []Symbol) []SearchHit {
	hits := make([]SearchHit, 0, len(symbols))
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
	return hits
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
				if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
					existing.hit.LineEnd = hit.LineEnd
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
				if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
					existing.hit.LineEnd = hit.LineEnd
				}
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
			if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
				existing.hit.LineEnd = hit.LineEnd
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
	results := make([]SearchHit, 0, len(merged))
	for _, hit := range merged {
		results = append(results, hit.hit)
	}
	return enrichMixedHits(paths, results)
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
