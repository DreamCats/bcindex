package bcindex

import "path/filepath"

const (
	defaultRelationEdgesPerKind = 5
	defaultDocLinksPerHit       = 5
)

func enrichMixedHits(paths RepoPaths, hits []SearchHit) ([]SearchHit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return hits, err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return hits, err
	}

	for i := range hits {
		hit := &hits[i]
		if hit.File == "" {
			continue
		}
		lineStart, lineEnd := normalizeLineRange(hit.Line, hit.LineEnd)

		links, err := store.ListDocLinksByFileRange(hit.File, lineStart, lineEnd, defaultDocLinksPerHit)
		if err != nil {
			return hits, err
		}
		if len(links) > 0 {
			hit.DocLinks = toDocLinkHits(links)
		}

		rels := make([]Relation, 0, defaultRelationEdgesPerKind*2)
		fileRels, err := store.ListRelationsByFile(hit.File, defaultRelationEdgesPerKind*4)
		if err != nil {
			return hits, err
		}
		rels = append(rels, fileRels...)

		dir := filepath.Dir(filepath.FromSlash(hit.File))
		dir = filepath.ToSlash(dir)
		if dir == "" {
			dir = "."
		}
		if dir != hit.File {
			dirRels, err := store.ListRelationsByFile(dir, defaultRelationEdgesPerKind*4)
			if err != nil {
				return hits, err
			}
			rels = append(rels, dirRels...)
		}
		hit.Relations = summarizeRelations(rels, defaultRelationEdgesPerKind)
	}
	return hits, nil
}

func normalizeLineRange(start, end int) (int, int) {
	if start <= 0 {
		start = 1
	}
	if end <= 0 {
		end = start
	}
	if end < start {
		end = start
	}
	return start, end
}

func summarizeRelations(rels []Relation, limit int) []RelationSummary {
	if len(rels) == 0 || limit <= 0 {
		return nil
	}
	byKind := make(map[string][]RelationEdge)
	order := make([]string, 0, 2)
	for _, rel := range rels {
		if rel.Kind == "" {
			continue
		}
		edges := byKind[rel.Kind]
		if len(edges) == 0 {
			order = append(order, rel.Kind)
		}
		if len(edges) >= limit {
			continue
		}
		edges = append(edges, RelationEdge{
			FromRef:    rel.FromRef,
			ToRef:      rel.ToRef,
			Line:       rel.Line,
			Source:     rel.Source,
			Confidence: rel.Confidence,
		})
		byKind[rel.Kind] = edges
	}
	if len(byKind) == 0 {
		return nil
	}
	summaries := make([]RelationSummary, 0, len(byKind))
	for _, kind := range order {
		edges := byKind[kind]
		if len(edges) == 0 {
			continue
		}
		summaries = append(summaries, RelationSummary{
			Kind:  kind,
			Edges: edges,
		})
	}
	return summaries
}

func toDocLinkHits(links []DocLink) []DocLinkHit {
	if len(links) == 0 {
		return nil
	}
	out := make([]DocLinkHit, 0, len(links))
	for _, link := range links {
		out = append(out, DocLinkHit{
			Symbol:     link.Symbol,
			Line:       link.Line,
			Source:     link.Source,
			Confidence: link.Confidence,
		})
	}
	return out
}
