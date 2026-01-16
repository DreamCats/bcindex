package retrieval

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DreamCats/bcindex/internal/store"
)

// EvidenceBuilder generates LLM-friendly evidence packs
type EvidenceBuilder struct {
	symbolStore  *store.SymbolStore
	packageStore *store.PackageStore
	edgeStore    *store.EdgeStore
	maxSnippets  int // Maximum number of code snippets
	maxLines     int // Maximum total lines across all snippets
	maxPackages  int // Maximum number of packages to include
	maxSymbols   int // Maximum number of symbols to include
}

// NewEvidenceBuilder creates a new evidence builder
func NewEvidenceBuilder(
	symbolStore *store.SymbolStore,
	packageStore *store.PackageStore,
	edgeStore *store.EdgeStore,
) *EvidenceBuilder {
	return &EvidenceBuilder{
		symbolStore:  symbolStore,
		packageStore: packageStore,
		edgeStore:    edgeStore,
		maxSnippets:  5,  // Default: up to 5 snippets
		maxLines:     200, // Default: up to 200 lines total
		maxPackages:  3,  // Default: top 3 packages
		maxSymbols:   10, // Default: top 10 symbols
	}
}

// Build generates an evidence pack from search results
func (b *EvidenceBuilder) Build(query string, results []SearchResult) (*store.EvidencePack, error) {
	pack := &store.EvidencePack{
		Query:       query,
		TopPackages: []store.PackageCard{},
		TopSymbols:  []store.SymbolCard{},
		GraphHints:  []string{},
		Snippets:    []store.CodeSnippet{},
		Metadata: store.PackMetadata{
			TotalSymbols:    len(results),
			TotalPackages:   0, // Will be updated
			TotalLines:      0, // Will be updated
			HasVectorSearch: len(results) > 0 && results[0].VectorScore > 0,
			GeneratedAt:     time.Now(),
		},
	}

	if len(results) == 0 {
		return pack, nil
	}

	// Step 1: Aggregate by package and build package cards
	pack.TopPackages = b.buildPackageCards(results)

	// Step 2: Build symbol cards from top results
	pack.TopSymbols = b.buildSymbolCards(results)

	// Step 3: Extract graph hints (call chains, relationships)
	pack.GraphHints = b.extractGraphHints(results)

	// Step 4: Extract code snippets (with strict line control)
	pack.Snippets = b.extractSnippets(results)

	// Update metadata
	pack.Metadata.TotalPackages = len(pack.TopPackages)
	pack.Metadata.TotalLines = b.countSnippetLines(pack.Snippets)

	return pack, nil
}

// buildPackageCards aggregates results by package
func (b *EvidenceBuilder) buildPackageCards(results []SearchResult) []store.PackageCard {
	// Group by package path
	pkgGroups := make(map[string][]SearchResult)
	for _, result := range results {
		pkgPath := result.Symbol.PackagePath
		pkgGroups[pkgPath] = append(pkgGroups[pkgPath], result)
	}

	// Score packages by: number of results + average score + graph features
	type scoredPackage struct {
		path  string
		score float64
	}
	scoredPackages := make([]scoredPackage, 0, len(pkgGroups))

	for pkgPath, group := range pkgGroups {
		score := 0.0
		for _, r := range group {
			score += float64(r.CombinedScore)
			// Boost for high graph scores
			if r.GraphScore > 0.5 {
				score += 0.2
			}
			// Boost for entry points
			if r.GraphFeatures != nil && r.GraphFeatures.IsEntry {
				score += 0.1
			}
		}

		scoredPackages = append(scoredPackages, scoredPackage{
			path:  pkgPath,
			score: score,
		})
	}

	// Sort by score and take top packages
	sort.Slice(scoredPackages, func(i, j int) bool {
		return scoredPackages[i].score > scoredPackages[j].score
	})

	// Limit to maxPackages
	if len(scoredPackages) > b.maxPackages {
		scoredPackages = scoredPackages[:b.maxPackages]
	}

	// Build package cards
	cards := make([]store.PackageCard, 0, len(scoredPackages))
	for _, sp := range scoredPackages {
		card := b.buildPackageCard(sp.path, pkgGroups[sp.path])
		cards = append(cards, card)
	}

	return cards
}

// buildPackageCard builds a single package card
func (b *EvidenceBuilder) buildPackageCard(pkgPath string, results []SearchResult) store.PackageCard {
	card := store.PackageCard{
		Path:        pkgPath,
		Role:        b.detectPackageRole(pkgPath, results),
		Summary:     b.generatePackageSummary(pkgPath, results),
		Why:         []string{},
		KeySymbols:  []string{},
		Imports:     []string{},
		ImportedBy:  []string{},
	}

	// Extract key symbols
	for _, r := range results {
		if len(card.KeySymbols) >= 5 { // Limit key symbols
			break
		}
		if r.Symbol.Exported || (r.GraphFeatures != nil && r.GraphFeatures.IsEntry) {
			card.KeySymbols = append(card.KeySymbols, r.Symbol.Name)
		}
	}

	// Generate reasons
	for _, r := range results {
		if len(card.Why) >= 3 {
			break
		}
		if r.CombinedScore > 0.7 {
			card.Why = append(card.Why, fmt.Sprintf("High relevance: %s", r.Symbol.Name))
		}
		if r.GraphFeatures != nil && r.GraphFeatures.IsEntry {
			card.Why = append(card.Why, "Contains entry points")
		}
		if r.GraphFeatures != nil && r.GraphFeatures.Layer == "service" {
			card.Why = append(card.Why, "Core service layer")
		}
	}

	// Load package info if available
	if b.packageStore != nil {
		if pkg, err := b.packageStore.Get(pkgPath); err == nil {
			card.Imports = pkg.Imports
			card.ImportedBy = pkg.ImportedBy
		}
	}

	return card
}

// detectPackageRole detects the architectural role of a package
func (b *EvidenceBuilder) detectPackageRole(pkgPath string, results []SearchResult) string {
	path := strings.ToLower(pkgPath)

	// Use layer detection from results
	for _, r := range results {
		if r.GraphFeatures != nil && r.GraphFeatures.Layer != "" {
			layer := r.GraphFeatures.Layer
			// Map layer to role
			switch layer {
			case "handler":
				return "interface/http"
			case "service":
				return "application/business"
			case "repository":
				return "infrastructure/persistence"
			case "domain":
				return "domain/model"
			case "middleware":
				return "infrastructure/middleware"
			}
		}
	}

	// Fallback to path-based detection
	if strings.Contains(path, "/handler/") || strings.Contains(path, "/controller/") {
		return "interface/http"
	}
	if strings.Contains(path, "/service/") || strings.Contains(path, "/usecase/") {
		return "application/business"
	}
	if strings.Contains(path, "/repository/") || strings.Contains(path, "/dao/") {
		return "infrastructure/persistence"
	}
	if strings.Contains(path, "/domain/") || strings.Contains(path, "/entity/") {
		return "domain/model"
	}

	return "application"
}

// generatePackageSummary generates a concise package summary
func (b *EvidenceBuilder) generatePackageSummary(pkgPath string, results []SearchResult) string {
	// Count symbol types
	typeCounts := make(map[string]int)
	for _, r := range results {
		typeCounts[r.Symbol.Kind]++
	}

	// Build summary
	parts := []string{}
	parts = append(parts, fmt.Sprintf("%d symbols", len(results)))

	for kind, count := range typeCounts {
		parts = append(parts, fmt.Sprintf("%d %s", count, kind))
	}

	// Add key functionality
	if len(results) > 0 {
		topResult := results[0]
		if topResult.Symbol.SemanticText != "" {
			// Use first 100 chars of semantic text
			summary := topResult.Symbol.SemanticText
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			parts = append(parts, summary)
		}
	}

	return strings.Join(parts, " | ")
}

// buildSymbolCards builds symbol cards from results
func (b *EvidenceBuilder) buildSymbolCards(results []SearchResult) []store.SymbolCard {
	// Limit to maxSymbols
	if len(results) > b.maxSymbols {
		results = results[:b.maxSymbols]
	}

	cards := make([]store.SymbolCard, 0, len(results))
	for _, r := range results {
		card := store.SymbolCard{
			ID:        r.Symbol.ID,
			Name:      r.Symbol.Name,
			Kind:      r.Symbol.Kind,
			Signature: r.Symbol.Signature,
			File:      r.Symbol.FilePath,
			Line:      r.Symbol.LineStart,
			Why:       r.Reason,
		}

		// Add snippet for top results only (first 3)
		if len(cards) < 3 && r.Symbol.SemanticText != "" {
			card.Snippet = r.Symbol.SemanticText
			// Limit snippet length
			if len(card.Snippet) > 300 {
				card.Snippet = card.Snippet[:300] + "..."
			}
		}

		cards = append(cards, card)
	}

	return cards
}

// extractGraphHints extracts relationship information
func (b *EvidenceBuilder) extractGraphHints(results []SearchResult) []string {
	hints := []string{}

	// Collect all symbol IDs
	symbolIDs := make([]string, 0, len(results))
	for _, r := range results {
		symbolIDs = append(symbolIDs, r.Symbol.ID)
	}

	// Find common callers (incoming edges) - only if edgeStore is available
	if b.edgeStore != nil {
		callerCounts := make(map[string]int)
		for _, id := range symbolIDs {
			incoming, _ := b.edgeStore.GetIncoming(id, "")
			for _, edge := range incoming {
				if edge.FromID != id { // Don't count self
					callerCounts[edge.FromID]++
				}
			}
		}

		// If there's a common caller, mention it
		if len(callerCounts) > 0 && b.symbolStore != nil {
			type callerCount struct {
				id    string
				count int
			}
			sortedCallers := make([]callerCount, 0, len(callerCounts))
			for id, count := range callerCounts {
				sortedCallers = append(sortedCallers, callerCount{id, count})
			}
			sort.Slice(sortedCallers, func(i, j int) bool {
				return sortedCallers[i].count > sortedCallers[j].count
			})

			// Get top common caller
			topCallerID := sortedCallers[0].id
			if sym, err := b.symbolStore.Get(topCallerID); err == nil {
				hints = append(hints, fmt.Sprintf("Common caller: %s (%s)", sym.Name, sym.PackagePath))
			}
		}
	}

	// Find entry points
	entryPoints := []string{}
	for _, r := range results {
		if r.GraphFeatures != nil && r.GraphFeatures.IsEntry {
			entryPoints = append(entryPoints, r.Symbol.Name)
		}
	}
	if len(entryPoints) > 0 {
		hints = append(hints, fmt.Sprintf("Entry points: %s", strings.Join(entryPoints, ", ")))
	}

	// Find highly connected symbols
	for _, r := range results {
		if r.GraphFeatures != nil && r.GraphFeatures.PageRank > 0.01 {
			hints = append(hints, fmt.Sprintf("Hub: %s (called by many)", r.Symbol.Name))
			break // Only mention one
		}
	}

	return hints
}

// extractSnippets extracts code snippets with strict line control
func (b *EvidenceBuilder) extractSnippets(results []SearchResult) []store.CodeSnippet {
	snippets := []store.CodeSnippet{}
	totalLines := 0

	// Prioritize: high score, exported, entry points
	type priorityResult struct {
		result   SearchResult
		priority float64
	}
	prioritized := make([]priorityResult, 0, len(results))

	for _, r := range results {
		priority := float64(r.CombinedScore)
		if r.Symbol.Exported {
			priority += 0.2
		}
		if r.GraphFeatures != nil && r.GraphFeatures.IsEntry {
			priority += 0.3
		}
		prioritized = append(prioritized, priorityResult{r, priority})
	}

	sort.Slice(prioritized, func(i, j int) bool {
		return prioritized[i].priority > prioritized[j].priority
	})

	// Extract snippets until we hit limits
	for _, pr := range prioritized {
		if len(snippets) >= b.maxSnippets {
			break
		}

		snippet := b.extractCodeSnippet(pr.result.Symbol)
		if snippet == nil {
			continue
		}

		// Calculate line count for this snippet
		snippetLines := snippet.EndLine - snippet.StartLine + 1

		// Check line limit
		if totalLines+snippetLines > b.maxLines {
			// Truncate snippet to fit
			remainingLines := b.maxLines - totalLines
			if remainingLines > 5 { // Only include if at least 5 lines
				truncatedContent := b.truncateToLines(snippet.Content, remainingLines)
				// Create truncated snippet
				truncatedSnippet := *snippet
				truncatedSnippet.Content = truncatedContent
				truncatedSnippet.EndLine = snippet.StartLine + remainingLines - 1
				truncatedSnippet.Reason = snippet.Reason + " (truncated)"
				snippets = append(snippets, truncatedSnippet)
			}
			break
		}

		snippets = append(snippets, *snippet)
		totalLines += snippetLines
	}

	return snippets
}

// extractCodeSnippet extracts a code snippet for a symbol
func (b *EvidenceBuilder) extractCodeSnippet(sym *store.Symbol) *store.CodeSnippet {
	if sym.FilePath == "" || sym.LineStart == 0 || sym.LineEnd == 0 {
		return nil
	}

	// Read file
	file, err := os.Open(sym.FilePath)
	if err != nil {
		// In test environment, file might not exist - return nil
		return nil
	}
	defer file.Close()

	// Extract relevant lines
	lines := []string{}
	scanner := bufio.NewScanner(file)
	currentLine := 1

	for scanner.Scan() {
		if currentLine >= sym.LineStart && currentLine <= sym.LineEnd {
			lines = append(lines, scanner.Text())
		}
		if currentLine > sym.LineEnd {
			break
		}
		currentLine++
	}

	if len(lines) == 0 {
		return nil
	}

	return &store.CodeSnippet{
		FilePath:   sym.FilePath,
		StartLine:  sym.LineStart,
		EndLine:    sym.LineEnd,
		Content:    strings.Join(lines, "\n"),
		Reason:     fmt.Sprintf("Symbol: %s (%s)", sym.Name, sym.Kind),
	}
}

// truncateToLines truncates content to specified number of lines
func (b *EvidenceBuilder) truncateToLines(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

// countSnippetLines counts total lines in all snippets
func (b *EvidenceBuilder) countSnippetLines(snippets []store.CodeSnippet) int {
	total := 0
	for _, s := range snippets {
		// Calculate lines from StartLine and EndLine
		total += s.EndLine - s.StartLine + 1
	}
	return total
}

// SetMaxSnippets sets the maximum number of snippets
func (b *EvidenceBuilder) SetMaxSnippets(max int) {
	b.maxSnippets = max
}

// SetMaxLines sets the maximum total lines
func (b *EvidenceBuilder) SetMaxLines(max int) {
	b.maxLines = max
}

// SetMaxPackages sets the maximum number of packages
func (b *EvidenceBuilder) SetMaxPackages(max int) {
	b.maxPackages = max
}

// SetMaxSymbols sets the maximum number of symbols
func (b *EvidenceBuilder) SetMaxSymbols(max int) {
	b.maxSymbols = max
}
