package retrieval

import (
	"math"
	"strings"

	"github.com/DreamCats/bcindex/internal/store"
)

// GraphRanker provides graph-based ranking using code relationship features
type GraphRanker struct {
	symbolStore *store.SymbolStore
	edgeStore   *store.EdgeStore
}

// NewGraphRanker creates a new graph ranker
func NewGraphRanker(
	symbolStore *store.SymbolStore,
	edgeStore *store.EdgeStore,
) *GraphRanker {
	return &GraphRanker{
		symbolStore: symbolStore,
		edgeStore:   edgeStore,
	}
}

// GraphFeatures represents graph-based features for a symbol
type GraphFeatures struct {
	InDegree      int     // Number of incoming edges (how many call this)
	OutDegree     int     // Number of outgoing edges (how many this calls)
	PageRank      float64 // PageRank score
	IsEntry       bool    // Is this an entry point (handler, main, etc.)
	IsInterface   bool    // Is this an interface
	Layer         string  // Architectural layer (handler, service, repo, etc.)
	Centrality    float64 // How central this node is in the graph
}

// RankedResult represents a result with graph-based ranking
type RankedResult struct {
	Symbol        *store.Symbol
	GraphScore    float64
	Features      *GraphFeatures
	OriginalScore float32 // The original combined score from hybrid search
	Reason        []string
}

// Rank ranks candidates based on graph features
func (r *GraphRanker) Rank(candidates []string, originalScores map[string]float32) ([]*RankedResult, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// Step 1: Compute features for all candidates
	features := make(map[string]*GraphFeatures)
	for _, id := range candidates {
		sym, err := r.symbolStore.Get(id)
		if err != nil || sym == nil {
			continue
		}

		features[id] = r.computeFeatures(sym)
	}

	// Step 2: Compute PageRank for all symbols in the graph
	pageRanks := r.computePageRank(candidates)

	// Update PageRank in features
	for id, pr := range pageRanks {
		if features[id] != nil {
			features[id].PageRank = pr
		}
	}

	// Step 3: Rank results
	results := make([]*RankedResult, 0, len(candidates))
	for _, id := range candidates {
		sym, err := r.symbolStore.Get(id)
		if err != nil || sym == nil {
			continue
		}

		feat := features[id]
		graphScore := r.computeGraphScore(feat)

		result := &RankedResult{
			Symbol:        sym,
			GraphScore:    graphScore,
			Features:      feat,
			OriginalScore: originalScores[id],
			Reason:        r.generateRankReasons(feat),
		}
		results = append(results, result)
	}

	return results, nil
}

// computeFeatures computes graph features for a symbol
func (r *GraphRanker) computeFeatures(sym *store.Symbol) *GraphFeatures {
	feat := &GraphFeatures{
		IsInterface: sym.Kind == store.KindInterface,
		Layer:       r.detectLayer(sym),
	}

	// Get incoming edges (calls, implements, embeds)
	incoming, _ := r.edgeStore.GetIncoming(sym.ID, "")
	for _, edge := range incoming {
		feat.InDegree += edge.Weight
	}

	// Get outgoing edges
	outgoing, _ := r.edgeStore.GetOutgoing(sym.ID, "")
	for _, edge := range outgoing {
		feat.OutDegree += edge.Weight
	}

	// Check if this is an entry point
	feat.IsEntry = r.isEntryPoint(sym)

	// Compute centrality (normalized in-degree + out-degree)
	totalDegree := feat.InDegree + feat.OutDegree
	if totalDegree > 0 {
		feat.Centrality = float64(totalDegree)
	}

	return feat
}

// computePageRank computes PageRank for symbols using the call graph
func (r *GraphRanker) computePageRank(symbolIDs []string) map[string]float64 {
	// Build adjacency list
	idSet := make(map[string]bool)
	for _, id := range symbolIDs {
		idSet[id] = true
	}

	// Get all edges for these symbols
	outgoing := make(map[string][]string) // id -> list of IDs it points to
	incoming := make(map[string][]string) // id -> list of IDs that point to it

	for _, id := range symbolIDs {
		edges, err := r.edgeStore.GetOutgoing(id, store.EdgeTypeCalls)
		if err != nil {
			continue
		}

		for _, edge := range edges {
			if idSet[edge.ToID] {
				outgoing[id] = append(outgoing[id], edge.ToID)
				incoming[edge.ToID] = append(incoming[edge.ToID], id)
			}
		}
	}

	// PageRank parameters
	damping := 0.85
	iterations := 20
	numNodes := len(symbolIDs)

	// Initialize PageRank uniformly
	pageRank := make(map[string]float64)
	initialRank := 1.0 / float64(numNodes)
	for _, id := range symbolIDs {
		pageRank[id] = initialRank
	}

	// Iterate
	for i := 0; i < iterations; i++ {
		newPageRank := make(map[string]float64)

		for _, id := range symbolIDs {
			// Get contributions from incoming links
			contribution := 0.0
			for _, source := range incoming[id] {
				outLinks := outgoing[source]
				if len(outLinks) > 0 {
					contribution += pageRank[source] / float64(len(outLinks))
				}
			}

			// PageRank formula: (1-d)/N + d * sum(PR(i)/Ci)
			newPageRank[id] = (1-damping)/float64(numNodes) + damping*contribution
		}

		pageRank = newPageRank
	}

	return pageRank
}

// computeGraphScore computes a composite score from graph features
func (r *GraphRanker) computeGraphScore(feat *GraphFeatures) float64 {
	score := 0.0

	// PageRank is the most important (0-1 range)
	score += feat.PageRank * 0.4

	// Normalized in-degree (being called is good)
	maxInDegree := 100.0 // Reasonable upper bound
	inDegreeScore := math.Min(float64(feat.InDegree)/maxInDegree, 1.0)
	score += inDegreeScore * 0.2

	// Entry points get a boost
	if feat.IsEntry {
		score += 0.2
	}

	// Interfaces get a boost (they're important abstractions)
	if feat.IsInterface {
		score += 0.1
	}

	// Layer-based scoring (prefer higher-level abstractions)
	layerScore := r.getLayerScore(feat.Layer)
	score += layerScore * 0.1

	return math.Min(score, 1.0)
}

// detectLayer detects the architectural layer from package path
func (r *GraphRanker) detectLayer(sym *store.Symbol) string {
	path := strings.ToLower(sym.PackagePath)

	// Common layer patterns
	if strings.Contains(path, "/handler/") || strings.Contains(path, "/controller/") ||
		strings.Contains(path, "/api/") || strings.Contains(path, "/http/") {
		return "handler"
	}

	if strings.Contains(path, "/service/") || strings.Contains(path, "/usecase/") ||
		strings.Contains(path, "/business/") {
		return "service"
	}

	if strings.Contains(path, "/repository/") || strings.Contains(path, "/repo/") ||
		strings.Contains(path, "/dao/") || strings.Contains(path, "/storage/") {
		return "repository"
	}

	if strings.Contains(path, "/domain/") || strings.Contains(path, "/entity/") ||
		strings.Contains(path, "/model/") {
		return "domain"
	}

	if strings.Contains(path, "/middleware/") || strings.Contains(path, "/filter/") {
		return "middleware"
	}

	if strings.Contains(path, "/util/") || strings.Contains(path, "/helper/") ||
		strings.Contains(path, "/common/") {
		return "util"
	}

	return "unknown"
}

// getLayerScore returns a score based on architectural layer
// Higher layers (handler, service) get higher scores for "design/architecture" queries
// Lower layers (repo, domain) get higher scores for "implementation/detail" queries
func (r *GraphRanker) getLayerScore(layer string) float64 {
	switch layer {
	case "handler":
		return 0.8
	case "service":
		return 0.9
	case "middleware":
		return 0.7
	case "domain":
		return 0.6
	case "repository":
		return 0.4
	case "util":
		return 0.3
	default:
		return 0.5
	}
}

// isEntryPoint determines if a symbol is an entry point to the system
func (r *GraphRanker) isEntryPoint(sym *store.Symbol) bool {
	// Check by name
	name := strings.ToLower(sym.Name)
	if name == "main" || strings.HasPrefix(name, "main.") ||
		strings.HasPrefix(name, "serve") || strings.HasPrefix(name, "start") ||
		strings.HasPrefix(name, "run") || strings.HasPrefix(name, "handle") {
		return true
	}

	// Check by kind and export status
	if sym.Exported && (sym.Kind == store.KindFunc || sym.Kind == store.KindMethod) {
		// Check if it has a signature suggesting HTTP handler
		signature := strings.ToLower(sym.Signature)
		if strings.Contains(signature, "http.responsewriter") ||
			strings.Contains(signature, "context.context") ||
			strings.Contains(signature, "gin.context") ||
			strings.Contains(signature, "fiber.ctx") {
			return true
		}
	}

	// Check package path for entry layer
	path := strings.ToLower(sym.PackagePath)
	if strings.Contains(path, "/handler/") || strings.Contains(path, "/controller/") ||
		strings.Contains(path, "/cmd/") || strings.Contains(path, "/api/") {
		return sym.Exported
	}

	return false
}

// generateRankReasons generates explanations for the ranking
func (r *GraphRanker) generateRankReasons(feat *GraphFeatures) []string {
	reasons := []string{}

	if feat.PageRank > 0.01 {
		reasons = append(reasons, "Highly connected in call graph")
	}

	if feat.InDegree > 10 {
		reasons = append(reasons, "Frequently called by other code")
	}

	if feat.IsEntry {
		reasons = append(reasons, "System entry point")
	}

	if feat.IsInterface {
		reasons = append(reasons, "Interface definition")
	}

	if feat.Layer == "service" || feat.Layer == "handler" {
		reasons = append(reasons, feat.Layer+" layer")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Graph feature match")
	}

	return reasons
}

// ReorderWithIntent reorders results based on query intent
func (r *GraphRanker) ReorderWithIntent(results []*RankedResult, query string) []*RankedResult {
	intent := r.detectIntent(query)
	if intent == "" {
		return results
	}

	// Sort based on intent
	sorted := make([]*RankedResult, len(results))
	copy(sorted, results)

	switch intent {
	case "design", "architecture":
		// Prefer service layer, interfaces
		sortByIntent(sorted, func(result *RankedResult) float64 {
			score := 0.0
			if result.Features.Layer == "service" {
				score += 1.0
			}
			if result.Features.IsInterface {
				score += 0.8
			}
			if result.Features.Layer == "handler" {
				score += 0.6
			}
			return score
		})

	case "implementation", "bug", "error":
		// Prefer concrete implementations, repos
		sortByIntent(sorted, func(result *RankedResult) float64 {
			score := 0.0
			if result.Features.Layer == "repository" {
				score += 1.0
			}
			if result.Features.Layer == "domain" {
				score += 0.8
			}
			if !result.Features.IsInterface {
				score += 0.5
			}
			return score
		})

	case "extension", "interface":
		// Prefer interfaces and middleware
		sortByIntent(sorted, func(result *RankedResult) float64 {
			score := 0.0
			if result.Features.IsInterface {
				score += 1.0
			}
			if result.Features.Layer == "middleware" {
				score += 0.8
			}
			return score
		})
	}

	return sorted
}

// detectIntent detects the user's intent from the query
func (r *GraphRanker) detectIntent(query string) string {
	q := strings.ToLower(query)

	// Design/architecture intent
	if strings.Contains(q, "design") || strings.Contains(q, "architecture") ||
		strings.Contains(q, "方案") || strings.Contains(q, "设计") ||
		strings.Contains(q, "structure") || strings.Contains(q, "模式") {
		return "design"
	}

	// Implementation/debugging intent
	if strings.Contains(q, "bug") || strings.Contains(q, "error") ||
		strings.Contains(q, "fix") || strings.Contains(q, "实现") ||
		strings.Contains(q, "问题") || strings.Contains(q, "调试") {
		return "implementation"
	}

	// Extension/interface intent
	if strings.Contains(q, "interface") || strings.Contains(q, "extend") ||
		strings.Contains(q, "插件") || strings.Contains(q, "接口") ||
		strings.Contains(q, "扩展") || strings.Contains(q, "extension") {
		return "extension"
	}

	return ""
}

// sortByIntent sorts results based on an intent scoring function
func sortByIntent(results []*RankedResult, scoreFunc func(*RankedResult) float64) {
	// Simple bubble sort for small lists
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			score1 := scoreFunc(results[j])
			score2 := scoreFunc(results[j+1])
			if score2 > score1 {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
