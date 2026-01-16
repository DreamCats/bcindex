package bcindex

import (
	"context"
	"fmt"
	"strings"
)

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
			LineEnd: hit.LineEnd,
			Score:   hit.Score,
			Snippet: snippet,
		})
	}
	return results
}
