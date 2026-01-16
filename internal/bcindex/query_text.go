package bcindex

import (
	"github.com/blevesearch/bleve/v2"
	blevequery "github.com/blevesearch/bleve/v2/search/query"
)

func queryText(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	return queryTextWithVariants(paths, meta, query, topK, true)
}

func queryTextContext(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	return queryTextWithVariants(paths, meta, query, topK, false)
}

func queryTextContextExact(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	return queryTextExact(paths, meta, query, topK, false)
}

func searchTextWithOptions(index bleve.Index, root string, query string, topK int, includePath bool) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 10
	}

	contentQuery := bleve.NewMatchQuery(query)
	contentQuery.SetField("content")
	contentQuery.SetBoost(1.0)
	titleQuery := bleve.NewMatchQuery(query)
	titleQuery.SetField("title")
	titleQuery.SetBoost(2.0)

	queries := []blevequery.Query{contentQuery, titleQuery}
	if includePath {
		pathQuery := bleve.NewMatchQuery(query)
		pathQuery.SetField("path")
		pathQuery.SetBoost(1.5)
		queries = append(queries, pathQuery)
	}
	disjunction := bleve.NewDisjunctionQuery(queries...)

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
		lineEnd := parseLineField(hit.Fields["line_end"])
		snippet := ""
		if lineStart > 0 {
			snippet = readLinesRange(root, pathVal, lineStart, lineEnd, 12)
		}
		if snippet == "" {
			lineStart, snippet = findMatchLine(root, pathVal, query)
			if snippet != "" {
				snippet = readLinesRange(root, pathVal, lineStart, lineStart+6, 12)
			}
		}
		hits = append(hits, SearchHit{
			Kind:    "text",
			Source:  "text",
			File:    pathVal,
			Line:    lineStart,
			LineEnd: lineEnd,
			Score:   hit.Score,
			Snippet: snippet,
		})
	}
	return hits, nil
}

func queryTextExact(paths RepoPaths, meta *RepoMeta, query string, topK int, includePath bool) ([]SearchHit, error) {
	index, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return nil, err
	}
	defer index.Close()

	limit := topK
	if limit <= 0 {
		limit = 10
	}
	return searchTextWithOptions(index, meta.Root, query, limit, includePath)
}

func queryTextWithVariants(paths RepoPaths, meta *RepoMeta, query string, topK int, includePath bool) ([]SearchHit, error) {
	index, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return nil, err
	}
	defer index.Close()

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
		hits, err := searchTextWithOptions(index, meta.Root, variant.Text, candidateTop, includePath)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			weighted = append(weighted, weightedHit{Hit: hit, Weight: variant.Weight})
		}
	}
	return mergeWeightedHits(weighted, limit), nil
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
