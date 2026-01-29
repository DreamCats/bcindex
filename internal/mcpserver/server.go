package mcpserver

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
		Name: "bcindex_context",
		Description: `Generate AI-friendly context (packages, symbols, snippets) for code understanding and implementation.

Filters:
- intent: Control result focus
  - "design": Prefer interfaces, service layer, architecture overview
  - "implementation": Prefer concrete code, repository/domain layer, details
  - "extension": Prefer interfaces, middleware, extension points
- kind_filter: Filter by symbol type (func, method, struct, interface, type)
- layer_filter: Filter by architectural layer (handler, service, repository, domain, middleware, util)

Use filters to get precise context for your task, reducing noise and token usage.`,
	}, s.evidenceTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "bcindex_refs",
		Description: `Query structural relationships between symbols.

Supported edge types:
- implements: Find types implementing an interface
- imports: Find packages importing a package
- embeds: Find types embedding a struct
- references: Find references to a symbol

NOTE: For function call hierarchy (who calls this function), use get_call_hierarchy from byte-lsp-mcp instead.`,
	}, s.refsTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "bcindex_read",
		Description: `Read source code content by symbol ID or file path.

Usage modes:
1. By symbol_id: Read the complete source code of a symbol (function, struct, etc.)
   - Use symbol IDs from bcindex_locate results
   - Automatically includes the full symbol definition

2. By file_path + line range: Read specific lines from a file
   - file_path is relative to repository root
   - start_line and end_line are 1-based

Options:
- context_lines: Add extra lines before/after for context
- max_lines: Limit output size (default: 500)
- include_line_no: Include line numbers in output (default: true)`,
	}, s.readTool)

	mcp.AddTool(server, &mcp.Tool{
		Name: "bcindex_status",
		Description: `Check the status of the bcindex for a repository.

Returns:
- Whether the repository is indexed
- Last index time and age
- Index statistics (symbols, packages, edges, embeddings)
- Staleness indicator (if index may be outdated)

Use this to verify index freshness before relying on search results.`,
	}, s.statusTool)

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

	expander, err := retrieval.LoadSynonymsForRepo(cfg.Repo.Path, cfg.Search.SynonymsFile)
	if err != nil {
		log.Printf("Warning: failed to load synonyms file: %v", err)
	}

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
		expander,
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

	expander, err := retrieval.LoadSynonymsForRepo(cfg.Repo.Path, cfg.Search.SynonymsFile)
	if err != nil {
		log.Printf("Warning: failed to load synonyms file: %v", err)
	}

	symbolStore, packageStore, edgeStore, vectorStore := idx.GetStores()
	retriever := retrieval.NewHybridRetriever(
		vectorStore,
		symbolStore,
		packageStore,
		edgeStore,
		idx.GetEmbedService(),
		expander,
	)

	evidenceBuilder := retriever.GetEvidenceBuilder()
	evidenceBuilder.SetMaxPackages(pickInt(input.MaxPackages, cfg.Evidence.MaxPackages))
	evidenceBuilder.SetMaxSymbols(pickInt(input.MaxSymbols, cfg.Evidence.MaxSymbols))
	evidenceBuilder.SetMaxSnippets(pickInt(input.MaxSnippets, cfg.Evidence.MaxSnippets))
	evidenceBuilder.SetMaxLines(pickInt(input.MaxLines, cfg.Evidence.MaxLines))

	opts := buildSearchOptions(cfg, input.TopK, input.IncludeUnexported, false, false)
	// Apply new filter options
	opts.Intent = input.Intent
	opts.Kinds = input.KindFilter
	opts.LayerFilter = input.LayerFilter
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

func (s *Server) readTool(ctx context.Context, _ *mcp.CallToolRequest, input ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
	if input.SymbolID == "" && input.FilePath == "" {
		return nil, ReadOutput{}, fmt.Errorf("symbol_id or file_path is required")
	}

	repoPath := input.Repo
	if repoPath == "" {
		repoPath = s.defaultRepo
	}

	cfg, err := prepareConfig(s.baseConfig, repoPath)
	if err != nil {
		return nil, ReadOutput{}, err
	}

	maxLines := input.MaxLines
	if maxLines <= 0 {
		maxLines = 500
	}

	includeLineNo := true
	if input.IncludeLineNo {
		includeLineNo = input.IncludeLineNo
	}

	// Mode 1: Read by symbol ID
	if input.SymbolID != "" {
		return s.readBySymbolID(cfg, input.SymbolID, input.ContextLines, maxLines, includeLineNo)
	}

	// Mode 2: Read by file path and line range
	return s.readByFilePath(cfg, input.FilePath, input.StartLine, input.EndLine, input.ContextLines, maxLines, includeLineNo)
}

func (s *Server) readBySymbolID(cfg *config.Config, symbolID string, contextLines int, maxLines int, includeLineNo bool) (*mcp.CallToolResult, ReadOutput, error) {
	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		return nil, ReadOutput{}, err
	}
	defer idx.Close()

	symbolStore, _, _, _ := idx.GetStores()
	sym, err := symbolStore.Get(symbolID)
	if err != nil {
		return nil, ReadOutput{}, err
	}
	if sym == nil {
		return nil, ReadOutput{}, fmt.Errorf("symbol not found: %s", symbolID)
	}

	// Resolve file path (stored as relative, need to make absolute)
	filePath := sym.FilePath
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cfg.Repo.Path, filePath)
	}

	startLine := sym.LineStart - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine := sym.LineEnd + contextLines

	content, actualStart, actualEnd, truncated, err := readFileLines(filePath, startLine, endLine, maxLines, includeLineNo)
	if err != nil {
		return nil, ReadOutput{}, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	output := ReadOutput{
		FilePath:   sym.FilePath, // Return relative path
		StartLine:  actualStart,
		EndLine:    actualEnd,
		TotalLines: actualEnd - actualStart + 1,
		Content:    content,
		Symbol: &ReadSymbol{
			ID:          sym.ID,
			Name:        sym.Name,
			Kind:        sym.Kind,
			Signature:   sym.Signature,
			PackagePath: sym.PackagePath,
		},
		Truncated: truncated,
	}

	return nil, output, nil
}

func (s *Server) readByFilePath(cfg *config.Config, filePath string, startLine int, endLine int, contextLines int, maxLines int, includeLineNo bool) (*mcp.CallToolResult, ReadOutput, error) {
	// Resolve file path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(cfg.Repo.Path, filePath)
	}

	// Validate file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, ReadOutput{}, fmt.Errorf("file not found: %s", filePath)
	}

	// Default to reading entire file if no line range specified
	if startLine <= 0 && endLine <= 0 {
		startLine = 1
		endLine = maxLines
	}

	// Apply context lines
	startLine = startLine - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine = endLine + contextLines

	content, actualStart, actualEnd, truncated, err := readFileLines(absPath, startLine, endLine, maxLines, includeLineNo)
	if err != nil {
		return nil, ReadOutput{}, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	output := ReadOutput{
		FilePath:   filePath, // Return as provided
		StartLine:  actualStart,
		EndLine:    actualEnd,
		TotalLines: actualEnd - actualStart + 1,
		Content:    content,
		Truncated:  truncated,
	}

	return nil, output, nil
}

// readFileLines reads specific lines from a file with optional line numbers.
func readFileLines(filePath string, startLine int, endLine int, maxLines int, includeLineNo bool) (content string, actualStart int, actualEnd int, truncated bool, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, 0, false, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	currentLine := 0
	linesRead := 0

	for scanner.Scan() {
		currentLine++

		if currentLine < startLine {
			continue
		}

		if currentLine > endLine || linesRead >= maxLines {
			if currentLine <= endLine {
				truncated = true
			}
			break
		}

		line := scanner.Text()
		if includeLineNo {
			lines = append(lines, fmt.Sprintf("%4d\t%s", currentLine, line))
		} else {
			lines = append(lines, line)
		}
		linesRead++

		if actualStart == 0 {
			actualStart = currentLine
		}
		actualEnd = currentLine
	}

	if err := scanner.Err(); err != nil {
		return "", 0, 0, false, err
	}

	if len(lines) == 0 {
		return "", startLine, startLine, false, nil
	}

	return strings.Join(lines, "\n"), actualStart, actualEnd, truncated, nil
}

func (s *Server) statusTool(ctx context.Context, _ *mcp.CallToolRequest, input StatusInput) (*mcp.CallToolResult, StatusOutput, error) {
	repoPath := input.Repo
	if repoPath == "" {
		repoPath = s.defaultRepo
	}

	cfg, err := prepareConfig(s.baseConfig, repoPath)
	if err != nil {
		return nil, StatusOutput{
			Indexed:      false,
			RootPath:     repoPath,
			DatabasePath: "",
			IsStale:      true,
			StaleReason:  fmt.Sprintf("Failed to prepare config: %v", err),
		}, nil
	}

	output := StatusOutput{
		Indexed:      false,
		RootPath:     cfg.Repo.Path,
		DatabasePath: cfg.Database.Path,
		IsStale:      true,
	}

	// Check database file
	dbInfo, err := os.Stat(cfg.Database.Path)
	if err != nil {
		if os.IsNotExist(err) {
			output.StaleReason = "Index database does not exist. Run 'bcindex index' to create it."
			return nil, output, nil
		}
		output.StaleReason = fmt.Sprintf("Cannot access database: %v", err)
		return nil, output, nil
	}

	output.Health = &IndexHealth{
		DatabaseExists: true,
		DatabaseSize:   dbInfo.Size(),
		DatabaseSizeStr: formatBytes(dbInfo.Size()),
	}

	// Open indexer to get repository info
	idx, err := indexer.NewIndexer(cfg)
	if err != nil {
		output.StaleReason = fmt.Sprintf("Failed to open index: %v", err)
		return nil, output, nil
	}
	defer idx.Close()

	// Get repository metadata
	repoStore := idx.GetRepoStore()
	repo, err := repoStore.GetByRootPath(cfg.Repo.Path)
	if err != nil {
		output.StaleReason = fmt.Sprintf("Failed to read repository info: %v", err)
		return nil, output, nil
	}

	if repo == nil || repo.LastIndexedAt == nil {
		output.StaleReason = "Repository not indexed. Run 'bcindex index' to index it."
		return nil, output, nil
	}

	// Repository is indexed
	output.Indexed = true
	output.LastIndexedAt = repo.LastIndexedAt.UTC().Format(time.RFC3339)
	output.Stats = &IndexStats{
		SymbolCount:   repo.SymbolCount,
		PackageCount:  repo.PackageCount,
		EdgeCount:     repo.EdgeCount,
		HasEmbeddings: repo.HasEmbeddings,
	}

	// Calculate index age and staleness
	indexAge := time.Since(*repo.LastIndexedAt)
	output.IndexAge = formatDuration(indexAge)

	// Check staleness (consider stale if > 24 hours)
	if indexAge > 24*time.Hour {
		output.IsStale = true
		output.StaleReason = fmt.Sprintf("Index is %s old. Consider re-indexing for latest changes.", output.IndexAge)
	} else if indexAge > 1*time.Hour {
		output.IsStale = false
		output.StaleReason = fmt.Sprintf("Index is %s old. Recent changes may not be reflected.", output.IndexAge)
	} else {
		output.IsStale = false
		output.StaleReason = ""
	}

	// Check if embeddings are available
	if !repo.HasEmbeddings {
		if output.StaleReason != "" {
			output.StaleReason += " "
		}
		output.StaleReason += "Warning: No embeddings available. Semantic search may be limited."
	}

	return nil, output, nil
}

// formatBytes formats bytes to human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatDuration formats duration to human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
	return fmt.Sprintf("%.1f days", d.Hours()/24)
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
	case store.EdgeTypeImplements,
		store.EdgeTypeImports,
		store.EdgeTypeReferences,
		store.EdgeTypeEmbeds:
		return true
	case store.EdgeTypeCalls:
		// calls edge type is not supported in bcindex_refs
		// use get_call_hierarchy from byte-lsp-mcp instead
		return false
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
