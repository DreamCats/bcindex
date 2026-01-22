package mcpserver

import "github.com/DreamCats/bcindex/internal/store"

// SearchInput defines inputs for the bcindex_locate MCP tool.
type SearchInput struct {
	Query             string `json:"query" jsonschema:"search query (natural language or keywords)"`
	Repo              string `json:"repo,omitempty" jsonschema:"repository root path (optional)"`
	TopK              int    `json:"top_k,omitempty" jsonschema:"number of results to return"`
	VectorOnly        bool   `json:"vector_only,omitempty" jsonschema:"use vector search only"`
	KeywordOnly       bool   `json:"keyword_only,omitempty" jsonschema:"use keyword search only"`
	IncludeUnexported bool   `json:"include_unexported,omitempty" jsonschema:"include unexported symbols"`
}

// SearchScores includes per-signal scores for a result.
type SearchScores struct {
	Vector   float32 `json:"vector"`
	Keyword  float32 `json:"keyword"`
	Graph    float64 `json:"graph"`
	Combined float32 `json:"combined"`
}

// SearchResultItem is a compact representation of a search result.
type SearchResultItem struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	PackagePath  string       `json:"package_path"`
	FilePath     string       `json:"file_path"`
	Line         int          `json:"line"`
	Signature    string       `json:"signature,omitempty"`
	DocComment   string       `json:"doc_comment,omitempty"`
	SemanticText string       `json:"semantic_text,omitempty"`
	Scores       SearchScores `json:"scores"`
	Reasons      []string     `json:"reasons,omitempty"`
}

// SearchOutput is the output for bcindex_locate.
type SearchOutput struct {
	Query   string             `json:"query"`
	Count   int                `json:"count"`
	Results []SearchResultItem `json:"results"`
}

// EvidenceInput defines inputs for the bcindex_context MCP tool.
type EvidenceInput struct {
	Query             string `json:"query" jsonschema:"search query for evidence pack"`
	Repo              string `json:"repo,omitempty" jsonschema:"repository root path (optional)"`
	TopK              int    `json:"top_k,omitempty" jsonschema:"number of results to search"`
	MaxPackages       int    `json:"max_packages,omitempty" jsonschema:"max packages to include"`
	MaxSymbols        int    `json:"max_symbols,omitempty" jsonschema:"max symbols to include"`
	MaxSnippets       int    `json:"max_snippets,omitempty" jsonschema:"max code snippets to include"`
	MaxLines          int    `json:"max_lines,omitempty" jsonschema:"max total lines across snippets"`
	IncludeUnexported bool   `json:"include_unexported,omitempty" jsonschema:"include unexported symbols"`
}

// EvidenceMetadata is MCP-friendly metadata with string timestamps.
type EvidenceMetadata struct {
	TotalSymbols    int    `json:"total_symbols"`
	TotalPackages   int    `json:"total_packages"`
	TotalLines      int    `json:"total_lines"`
	HasVectorSearch bool   `json:"has_vector_search"`
	GeneratedAt     string `json:"generated_at"`
}

// EvidenceOutput mirrors store.EvidencePack but uses string timestamps.
type EvidenceOutput struct {
	Query       string              `json:"query"`
	TopPackages []store.PackageCard `json:"top_packages"`
	TopSymbols  []store.SymbolCard  `json:"top_symbols"`
	GraphHints  []string            `json:"graph_hints"`
	Snippets    []store.CodeSnippet `json:"snippets"`
	Metadata    EvidenceMetadata    `json:"metadata"`
}

// RefsInput defines inputs for the bcindex_refs MCP tool.
type RefsInput struct {
	SymbolID    string `json:"symbol_id,omitempty" jsonschema:"symbol id (preferred)"`
	SymbolName  string `json:"symbol_name,omitempty" jsonschema:"symbol name (exact match)"`
	PackagePath string `json:"package_path,omitempty" jsonschema:"filter by package path (optional)"`
	Repo        string `json:"repo,omitempty" jsonschema:"repository root path (optional)"`
	EdgeType    string `json:"edge_type,omitempty" jsonschema:"calls|references|implements|imports|embeds"`
	Direction   string `json:"direction,omitempty" jsonschema:"incoming|outgoing|both"`
	TopK        int    `json:"top_k,omitempty" jsonschema:"max edges to return"`
}

// RefSymbol is a compact symbol representation for refs output.
type RefSymbol struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	PackagePath string `json:"package_path"`
	FilePath    string `json:"file_path"`
	Line        int    `json:"line"`
	Signature   string `json:"signature,omitempty"`
}

// RefEndpoint represents one side of a reference edge.
type RefEndpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	PackagePath string `json:"package_path"`
	FilePath    string `json:"file_path"`
	Line        int    `json:"line"`
}

// RefEdge is the MCP-friendly representation of a relationship.
type RefEdge struct {
	EdgeType   string      `json:"edge_type"`
	From       RefEndpoint `json:"from"`
	To         RefEndpoint `json:"to"`
	ImportPath string      `json:"import_path,omitempty"`
}

// RefsOutput is the output for bcindex_refs.
type RefsOutput struct {
	SymbolID   string      `json:"symbol_id,omitempty"`
	SymbolName string      `json:"symbol_name,omitempty"`
	Direction  string      `json:"direction,omitempty"`
	EdgeType   string      `json:"edge_type,omitempty"`
	Count      int         `json:"count"`
	Symbols    []RefSymbol `json:"symbols"`
	Edges      []RefEdge   `json:"edges"`
}
