package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DreamCats/bcindex/internal/config"
	"github.com/DreamCats/bcindex/internal/indexer"
	"github.com/DreamCats/bcindex/internal/retrieval"
	"github.com/DreamCats/bcindex/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server exposes bcindex search/evidence via MCP stdio.
type Server struct {
	baseConfig  *config.Config
	defaultRepo string
	version     string
}

// New creates a new MCP server wrapper.
func New(baseConfig *config.Config, defaultRepo string, version string) *Server {
	return &Server{
		baseConfig:  baseConfig,
		defaultRepo: defaultRepo,
		version:     version,
	}
}

// Run starts the MCP stdio server.
func (s *Server) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "bcindex",
		Title:   "BCIndex",
		Version: s.version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "bcindex_locate",
		Description: "Locate symbols, files, or APIs (quick lookup for definitions/usages).",
	}, s.searchTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "bcindex_context",
		Description: "Provide richer context (packages, symbols, snippets) for implementation/flow questions.",
	}, s.evidenceTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "bcindex_refs",
		Description: "List references/callers/dependencies for a symbol (incoming/outgoing edges).",
	}, s.refsTool)

	return server.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) searchTool(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	if input.Query == "" {
		return nil, SearchOutput{}, fmt.Errorf("query is required")
	}
	if input.VectorOnly && input.KeywordOnly {
		return nil, SearchOutput{}, fmt.Errorf("vector_only and keyword_only cannot both be true")
	}

	repoPath := input.Repo
	if repoPath == "" {
		repoPath = s.defaultRepo
	}

	cfg, err := prepareConfig(s.baseConfig, repoPath)
	if err != nil {
		return nil, SearchOutput{}, err
	}

	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	defer idx.Close()

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
	)

	opts := buildSearchOptions(cfg, input.TopK, input.IncludeUnexported, input.VectorOnly, input.KeywordOnly)
	results, err := retriever.Search(ctx, input.Query, opts)
	if err != nil {
		return nil, SearchOutput{}, err
	}

	output := SearchOutput{
		Query:   input.Query,
		Count:   len(results),
		Results: mapSearchResults(results),
	}
	return nil, output, nil
}

func (s *Server) evidenceTool(ctx context.Context, _ *mcp.CallToolRequest, input EvidenceInput) (*mcp.CallToolResult, EvidenceOutput, error) {
	if input.Query == "" {
		return nil, EvidenceOutput{}, fmt.Errorf("query is required")
	}

	repoPath := input.Repo
	if repoPath == "" {
		repoPath = s.defaultRepo
	}

	cfg, err := prepareConfig(s.baseConfig, repoPath)
	if err != nil {
		return nil, EvidenceOutput{}, err
	}

	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		return nil, EvidenceOutput{}, err
	}
	defer idx.Close()

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
	)

	evidenceBuilder := retriever.GetEvidenceBuilder()
	evidenceBuilder.SetMaxPackages(pickInt(input.MaxPackages, cfg.Evidence.MaxPackages))
	evidenceBuilder.SetMaxSymbols(pickInt(input.MaxSymbols, cfg.Evidence.MaxSymbols))
	evidenceBuilder.SetMaxSnippets(pickInt(input.MaxSnippets, cfg.Evidence.MaxSnippets))
	evidenceBuilder.SetMaxLines(pickInt(input.MaxLines, cfg.Evidence.MaxLines))

	opts := buildSearchOptions(cfg, input.TopK, input.IncludeUnexported, false, false)
	pack, err := retriever.SearchAsEvidencePack(ctx, input.Query, opts)
	if err != nil {
		return nil, EvidenceOutput{}, err
	}

	output := toEvidenceOutput(pack)
	return nil, output, nil
}

func (s *Server) refsTool(ctx context.Context, _ *mcp.CallToolRequest, input RefsInput) (*mcp.CallToolResult, RefsOutput, error) {
	if input.SymbolID == "" && input.SymbolName == "" {
		return nil, RefsOutput{}, fmt.Errorf("symbol_id or symbol_name is required")
	}

	repoPath := input.Repo
	if repoPath == "" {
		repoPath = s.defaultRepo
	}

	cfg, err := prepareConfig(s.baseConfig, repoPath)
	if err != nil {
		return nil, RefsOutput{}, err
	}

	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		return nil, RefsOutput{}, err
	}
	defer idx.Close()

	symbolStore, _, edgeStore, _ := idx.GetStores()

	var symbols []*store.Symbol
	if input.SymbolID != "" {
		sym, err := symbolStore.Get(input.SymbolID)
		if err != nil {
			return nil, RefsOutput{}, err
		}
		if sym != nil {
			symbols = []*store.Symbol{sym}
		}
	} else {
		limit := input.TopK
		if limit <= 0 {
			limit = 20
		}
		symbols, err = symbolStore.FindByName(input.SymbolName, cfg.Repo.Path, input.PackagePath, limit)
		if err != nil {
			return nil, RefsOutput{}, err
		}
	}

	output := RefsOutput{
		SymbolID:   input.SymbolID,
		SymbolName: input.SymbolName,
		Direction:  normalizeDirection(input.Direction),
		EdgeType:   strings.TrimSpace(input.EdgeType),
		Symbols:    mapRefSymbols(symbols),
		Edges:      []RefEdge{},
		Count:      0,
	}

	if len(symbols) == 0 {
		return nil, output, nil
	}

	direction := output.Direction
	if direction == "" {
		direction = "incoming"
		output.Direction = direction
	}
	if direction != "incoming" && direction != "outgoing" && direction != "both" {
		return nil, RefsOutput{}, fmt.Errorf("invalid direction: %s", direction)
	}

	edgeType := output.EdgeType
	if edgeType != "" && !isValidEdgeType(edgeType) {
		return nil, RefsOutput{}, fmt.Errorf("invalid edge_type: %s", edgeType)
	}

	edgeMap := make(map[string]RefEdge)
	for _, sym := range symbols {
		if sym == nil {
			continue
		}

		if direction == "incoming" || direction == "both" {
			incoming, err := edgeStore.GetIncoming(sym.ID, edgeType)
			if err != nil {
				return nil, RefsOutput{}, err
			}
			collectRefEdges(edgeMap, incoming, symbolStore)
		}

		if direction == "outgoing" || direction == "both" {
			outgoing, err := edgeStore.GetOutgoing(sym.ID, edgeType)
			if err != nil {
				return nil, RefsOutput{}, err
			}
			collectRefEdges(edgeMap, outgoing, symbolStore)
		}
	}

	edges := make([]RefEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, edge)
	}

	if input.TopK > 0 && len(edges) > input.TopK {
		edges = edges[:input.TopK]
	}

	output.Edges = edges
	output.Count = len(edges)

	return nil, output, nil
}

func buildSearchOptions(cfg *config.Config, topK int, includeUnexported bool, vectorOnly bool, keywordOnly bool) retrieval.SearchOptions {
	opts := retrieval.DefaultSearchOptions()
	opts.TopK = cfg.Search.DefaultTopK
	opts.VectorWeight = cfg.Search.VectorWeight
	opts.KeywordWeight = cfg.Search.KeywordWeight
	opts.GraphWeight = cfg.Search.GraphWeight
	opts.EnableGraphRank = cfg.Search.EnableGraphRank

	if topK > 0 {
		opts.TopK = topK
	}
	if includeUnexported {
		opts.ExportedOnly = false
	}
	if vectorOnly {
		opts.VectorWeight = 1.0
		opts.KeywordWeight = 0.0
		opts.GraphWeight = 0.0
	}
	if keywordOnly {
		opts.VectorWeight = 0.0
		opts.KeywordWeight = 1.0
		opts.GraphWeight = 0.0
	}
	return opts
}

func mapSearchResults(results []retrieval.SearchResult) []SearchResultItem {
	items := make([]SearchResultItem, 0, len(results))
	for _, result := range results {
		if result.Symbol == nil {
			continue
		}
		sym := result.Symbol
		items = append(items, SearchResultItem{
			ID:           sym.ID,
			Name:         sym.Name,
			Kind:         sym.Kind,
			PackagePath:  sym.PackagePath,
			FilePath:     sym.FilePath,
			Line:         sym.LineStart,
			Signature:    sym.Signature,
			DocComment:   sym.DocComment,
			SemanticText: sym.SemanticText,
			Scores: SearchScores{
				Vector:   result.VectorScore,
				Keyword:  result.KeywordScore,
				Graph:    result.GraphScore,
				Combined: result.CombinedScore,
			},
			Reasons: result.Reason,
		})
	}
	return items
}

func toEvidenceOutput(pack *store.EvidencePack) EvidenceOutput {
	if pack == nil {
		return EvidenceOutput{
			TopPackages: []store.PackageCard{},
			TopSymbols:  []store.SymbolCard{},
			GraphHints:  []string{},
			Snippets:    []store.CodeSnippet{},
		}
	}

	topPackages := make([]store.PackageCard, 0, len(pack.TopPackages))
	for _, pkg := range pack.TopPackages {
		pkg.Why = ensureStringSlice(pkg.Why)
		pkg.KeySymbols = ensureStringSlice(pkg.KeySymbols)
		pkg.Imports = ensureStringSlice(pkg.Imports)
		pkg.ImportedBy = ensureStringSlice(pkg.ImportedBy)
		topPackages = append(topPackages, pkg)
	}

	topSymbols := make([]store.SymbolCard, 0, len(pack.TopSymbols))
	for _, sym := range pack.TopSymbols {
		sym.Why = ensureStringSlice(sym.Why)
		topSymbols = append(topSymbols, sym)
	}

	graphHints := ensureStringSlice(pack.GraphHints)
	snippets := pack.Snippets
	if snippets == nil {
		snippets = []store.CodeSnippet{}
	}

	return EvidenceOutput{
		Query:       pack.Query,
		TopPackages: topPackages,
		TopSymbols:  topSymbols,
		GraphHints:  graphHints,
		Snippets:    snippets,
		Metadata: EvidenceMetadata{
			TotalSymbols:    pack.Metadata.TotalSymbols,
			TotalPackages:   pack.Metadata.TotalPackages,
			TotalLines:      pack.Metadata.TotalLines,
			HasVectorSearch: pack.Metadata.HasVectorSearch,
			GeneratedAt:     pack.Metadata.GeneratedAt.UTC().Format(time.RFC3339),
		},
	}
}

func normalizeDirection(direction string) string {
	if direction == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(direction))
}

func isValidEdgeType(edgeType string) bool {
	switch edgeType {
	case store.EdgeTypeCalls,
		store.EdgeTypeImplements,
		store.EdgeTypeImports,
		store.EdgeTypeReferences,
		store.EdgeTypeEmbeds:
		return true
	default:
		return false
	}
}

func mapRefSymbols(symbols []*store.Symbol) []RefSymbol {
	if len(symbols) == 0 {
		return []RefSymbol{}
	}
	out := make([]RefSymbol, 0, len(symbols))
	for _, sym := range symbols {
		if sym == nil {
			continue
		}
		out = append(out, RefSymbol{
			ID:          sym.ID,
			Name:        sym.Name,
			Kind:        sym.Kind,
			PackagePath: sym.PackagePath,
			FilePath:    sym.FilePath,
			Line:        sym.LineStart,
			Signature:   sym.Signature,
		})
	}
	return out
}

func collectRefEdges(edgeMap map[string]RefEdge, edges []*store.Edge, symbolStore *store.SymbolStore) {
	for _, edge := range edges {
		if edge == nil {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s", edge.FromID, edge.ToID, edge.EdgeType)
		if _, exists := edgeMap[key]; exists {
			continue
		}

		fromSym, _ := symbolStore.Get(edge.FromID)
		toSym, _ := symbolStore.Get(edge.ToID)
		if fromSym == nil || toSym == nil {
			continue
		}

		edgeMap[key] = RefEdge{
			EdgeType: edge.EdgeType,
			From: RefEndpoint{
				ID:          fromSym.ID,
				Name:        fromSym.Name,
				Kind:        fromSym.Kind,
				PackagePath: fromSym.PackagePath,
				FilePath:    fromSym.FilePath,
				Line:        fromSym.LineStart,
			},
			To: RefEndpoint{
				ID:          toSym.ID,
				Name:        toSym.Name,
				Kind:        toSym.Kind,
				PackagePath: toSym.PackagePath,
				FilePath:    toSym.FilePath,
				Line:        toSym.LineStart,
			},
			ImportPath: edge.ImportPath,
		}
	}
}

func pickInt(input int, fallback int) int {
	if input > 0 {
		return input
	}
	return fallback
}

func ensureStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
