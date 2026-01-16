package retrieval

import (
	"context"
	"fmt"
	"sort"

	"github.com/DreamCats/bcindex/internal/embedding"
	"github.com/DreamCats/bcindex/internal/store"
)

// HybridRetriever provides hybrid search combining vector and keyword search
type HybridRetriever struct {
	vectorStore    *store.VectorStore
	symbolStore    *store.SymbolStore
	packageStore   *store.PackageStore
	edgeStore      *store.EdgeStore
	embedService   *embedding.Service
	graphRanker    *GraphRanker
	evidenceBuilder *EvidenceBuilder
}

// NewHybridRetriever creates a new hybrid retriever
func NewHybridRetriever(
	vectorStore *store.VectorStore,
	symbolStore *store.SymbolStore,
	packageStore *store.PackageStore,
	edgeStore *store.EdgeStore,
	embedService *embedding.Service,
) *HybridRetriever {
	ranker := NewGraphRanker(symbolStore, edgeStore)
	evidenceBuilder := NewEvidenceBuilder(symbolStore, packageStore, edgeStore)

	return &HybridRetriever{
		vectorStore:     vectorStore,
		symbolStore:     symbolStore,
		packageStore:    packageStore,
		edgeStore:       edgeStore,
		embedService:    embedService,
		graphRanker:     ranker,
		evidenceBuilder: evidenceBuilder,
	}
}

// SearchOptions configures search behavior
type SearchOptions struct {
	TopK              int     // Number of results to return
	VectorWeight      float32 // Weight for vector similarity (0-1)
	KeywordWeight     float32 // Weight for keyword search (0-1)
	GraphWeight       float32 // Weight for graph-based ranking (0-1)
	ExportedOnly      bool    // Only return exported symbols
	Kinds             []string // Filter by symbol kinds
	PackagePath       string   // Filter by package path
	IncludePackages   bool    // Also return package-level results
	EnableGraphRank   bool    // Enable graph-based ranking
}

// DefaultSearchOptions returns default search options
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		TopK:            10,
		VectorWeight:    0.6,
		KeywordWeight:   0.2,
		GraphWeight:     0.2,
		ExportedOnly:    true,
		Kinds:           nil,
		PackagePath:     "",
		IncludePackages: false,
		EnableGraphRank: true,
	}
}

// SearchResult represents a combined search result
type SearchResult struct {
	Symbol        *store.Symbol
	Package       *store.Package // Populated if IncludePackages is true
	VectorScore   float32        // Vector similarity score
	KeywordScore  float32        // Keyword search score
	GraphScore    float64        // Graph-based ranking score
	CombinedScore float32        // Final combined score
	Reason        []string       // Explanation of why this result was returned
	GraphFeatures *GraphFeatures // Graph-based features (if graph ranking enabled)
}

// Search performs hybrid search
func (h *HybridRetriever) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}

	// Normalize weights
	totalWeight := opts.VectorWeight + opts.KeywordWeight
	if totalWeight == 0 {
		// Default to vector-only if both weights are 0
		opts.VectorWeight = 1.0
		totalWeight = 1.0
	}
	opts.VectorWeight /= totalWeight
	opts.KeywordWeight /= totalWeight

	// Step 1: Generate query embedding
	var queryVector []float32
	if opts.VectorWeight > 0 {
		var err error
		queryVector, err = h.embedService.Embed(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to embed query: %w", err)
		}
	}

	// Step 2: Vector search
	vectorResults := make(map[string]*scoredSymbol)
	if opts.VectorWeight > 0 && queryVector != nil {
		vResults, err := h.vectorStore.Search(queryVector, opts.TopK*2, h.symbolStore)
		if err != nil {
			return nil, fmt.Errorf("vector search failed: %w", err)
		}

		for _, r := range vResults {
			vectorResults[r.SymbolID] = &scoredSymbol{
				symbol: r.Symbol,
				score:  r.Score,
			}
		}
	}

	// Step 3: Keyword search using FTS
	keywordResults := make(map[string]*scoredSymbol)
	if opts.KeywordWeight > 0 {
		kResults, err := h.symbolStore.SearchFTS(query, opts.TopK*2)
		if err != nil {
			return nil, fmt.Errorf("keyword search failed: %w", err)
		}

		for i, sym := range kResults {
			// Simple scoring based on rank
			score := float32(1.0 - float64(i)/float64(len(kResults)))
			keywordResults[sym.ID] = &scoredSymbol{
				symbol: sym,
				score:  score,
			}
		}
	}

	// Step 4: Combine scores
	combinedScores := make(map[string]*combinedResult)

	// Add vector results
	for symbolID, result := range vectorResults {
		combinedScores[symbolID] = &combinedResult{
			symbol:       result.symbol,
			vectorScore:  result.score,
			keywordScore: 0,
		}
	}

	// Add keyword results and combine
	for symbolID, result := range keywordResults {
		if existing, ok := combinedScores[symbolID]; ok {
			existing.keywordScore = result.score
		} else {
			combinedScores[symbolID] = &combinedResult{
				symbol:       result.symbol,
				vectorScore:  0,
				keywordScore: result.score,
			}
		}
	}

	// Step 5: Apply filters and compute final scores
	var results []SearchResult
	for _, combined := range combinedScores {
		// Apply filters
		if opts.ExportedOnly && !combined.symbol.Exported {
			continue
		}

		if len(opts.Kinds) > 0 {
			kindMatch := false
			for _, kind := range opts.Kinds {
				if combined.symbol.Kind == kind {
					kindMatch = true
					break
				}
			}
			if !kindMatch {
				continue
			}
		}

		if opts.PackagePath != "" && combined.symbol.PackagePath != opts.PackagePath {
			continue
		}

		// Compute combined score
		finalScore := opts.VectorWeight*combined.vectorScore + opts.KeywordWeight*combined.keywordScore

		result := SearchResult{
			Symbol:        combined.symbol,
			VectorScore:   combined.vectorScore,
			KeywordScore:  combined.keywordScore,
			CombinedScore: finalScore,
			Reason:        h.generateReasons(combined),
		}

		results = append(results, result)
	}

	// Step 6: Apply graph-based ranking if enabled
	if opts.EnableGraphRank && len(results) > 0 {
		results = h.applyGraphRanking(results, query, opts)
	}

	// Step 7: Sort by combined score
	sort.Slice(results, func(i, j int) bool {
		return results[i].CombinedScore > results[j].CombinedScore
	})

	// Keep top K
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	// Step 8: Optionally load package information
	if opts.IncludePackages {
		for i := range results {
			if pkg, err := h.packageStore.Get(results[i].Symbol.PackagePath); err == nil {
				results[i].Package = pkg
			}
		}
	}

	return results, nil
}

// scoredSymbol holds a symbol with its score
type scoredSymbol struct {
	symbol *store.Symbol
	score  float32
}

// combinedResult holds combined search information
type combinedResult struct {
	symbol       *store.Symbol
	vectorScore  float32
	keywordScore float32
}

// generateReasons generates explanation for why a result was returned
func (h *HybridRetriever) generateReasons(combined *combinedResult) []string {
	var reasons []string

	if combined.vectorScore > 0.7 {
		reasons = append(reasons, "Strong semantic similarity")
	} else if combined.vectorScore > 0.5 {
		reasons = append(reasons, "Moderate semantic similarity")
	}

	if combined.keywordScore > 0.7 {
		reasons = append(reasons, "Exact keyword match")
	} else if combined.keywordScore > 0.5 {
		reasons = append(reasons, "Partial keyword match")
	}

	if combined.symbol.Exported {
		reasons = append(reasons, "Exported API")
	}

	if combined.symbol.Kind == "func" || combined.symbol.Kind == "method" {
		reasons = append(reasons, "Executable code")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Match found")
	}

	return reasons
}

// applyGraphRanking applies graph-based ranking to results
func (h *HybridRetriever) applyGraphRanking(results []SearchResult, query string, opts SearchOptions) []SearchResult {
	// Extract candidate IDs and original scores
	candidateIDs := make([]string, len(results))
	originalScores := make(map[string]float32)

	for i, result := range results {
		candidateIDs[i] = result.Symbol.ID
		originalScores[result.Symbol.ID] = result.CombinedScore
	}

	// Rank using graph features
	rankedResults, err := h.graphRanker.Rank(candidateIDs, originalScores)
	if err != nil || len(rankedResults) == 0 {
		return results // Fallback to original results
	}

	// Reorder based on intent
	rankedResults = h.graphRanker.ReorderWithIntent(rankedResults, query)

	// Update results with graph scores
	resultMap := make(map[string]*SearchResult)
	for i := range results {
		resultMap[results[i].Symbol.ID] = &results[i]
	}

	// Create new results list with graph-based ordering
	newResults := make([]SearchResult, 0, len(results))
	for _, ranked := range rankedResults {
		if original, ok := resultMap[ranked.Symbol.ID]; ok {
			// Combine original score with graph score
			totalWeight := float32(1) - opts.GraphWeight
			if totalWeight <= 0 {
				totalWeight = 0.5
			}

			// Normalize weights
			vecKeywordWeight := 1.0 - opts.GraphWeight

			combinedScore := float32(original.CombinedScore)*vecKeywordWeight +
				float32(ranked.GraphScore)*opts.GraphWeight

			result := SearchResult{
				Symbol:        ranked.Symbol,
				VectorScore:   original.VectorScore,
				KeywordScore:  original.KeywordScore,
				GraphScore:    ranked.GraphScore,
				CombinedScore: combinedScore,
				GraphFeatures: ranked.Features,
				Reason:        h.mergeReasons(original.Reason, ranked.Reason),
			}

			newResults = append(newResults, result)
		}
	}

	return newResults
}

// mergeReasons merges original and graph-based reasons
func (h *HybridRetriever) mergeReasons(original, graphReasons []string) []string {
	seen := make(map[string]bool)
	merged := make([]string, 0)

	// Add original reasons first
	for _, r := range original {
		if !seen[r] {
			merged = append(merged, r)
			seen[r] = true
		}
	}

	// Add graph reasons
	for _, r := range graphReasons {
		if !seen[r] {
			merged = append(merged, r)
			seen[r] = true
		}
	}

	return merged
}

// SearchAsEvidencePack performs search and returns an evidence pack
func (h *HybridRetriever) SearchAsEvidencePack(ctx context.Context, query string, opts SearchOptions) (*store.EvidencePack, error) {
	// Perform regular search
	results, err := h.Search(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Build evidence pack from results
	pack, err := h.evidenceBuilder.Build(query, results)
	if err != nil {
		return nil, fmt.Errorf("failed to build evidence pack: %w", err)
	}

	return pack, nil
}

// GetEvidenceBuilder returns the evidence builder for configuration
func (h *HybridRetriever) GetEvidenceBuilder() *EvidenceBuilder {
	return h.evidenceBuilder
}
