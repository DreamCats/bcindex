package store

import "time"

// Symbol represents a semantic unit in the codebase
// Corresponds to: package, file, interface, struct, func, method, const, var, field
type Symbol struct {
	// Primary key
	ID string `json:"id"`

	// Repository identification
	RepoPath string `json:"repo_path"`

	// Symbol classification
	Kind string `json:"kind"` // package | file | interface | struct | func | method | const | var | field

	// Package information
	PackagePath string `json:"package_path"` // Full package path (e.g., github.com/user/repo/pkg)
	PackageName string `json:"package_name"` // Short package name (e.g., pkg)

	// Symbol identification
	Name      string `json:"name"`      // Symbol name (e.g., MyFunc, MyType)
	Signature string `json:"signature"` // Full signature (for funcs/methods)

	// Source location
	FilePath  string `json:"file_path"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`

	// Documentation
	DocComment string `json:"doc_comment"`

	// Visibility
	Exported bool `json:"exported"`

	// Semantic information (core field for RAG)
	SemanticText string `json:"semantic_text"` // Generated description of role/responsibilities

	// Vector embedding (optional)
	Embedding []float32 `json:"-"` // Not stored in JSON, managed separately

	// Search hints
	Tokens []string `json:"tokens"` // Keywords for better matching

	// Type-specific fields
	TypeDetails *TypeDetails `json:"type_details,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TypeDetails contains additional information for type-level symbols
type TypeDetails struct {
	// For interfaces
	IsInterface      bool     `json:"is_interface"`
	InterfaceMethods []string `json:"interface_methods,omitempty"`

	// For structs
	IsStruct      bool     `json:"is_struct"`
	EmbeddedTypes []string `json:"embedded_types,omitempty"`
	Fields        []Field  `json:"fields,omitempty"`

	// For functions/methods
	IsFunc       bool     `json:"is_func"`
	IsMethod     bool     `json:"is_method"`
	ReceiverType string   `json:"receiver_type,omitempty"` // For methods
	Params       []Param  `json:"params,omitempty"`
	Returns      []string `json:"returns,omitempty"`
}

// Field represents a struct field
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
}

// Param represents a function parameter
type Param struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Edge represents a relationship between two symbols
type Edge struct {
	FromID   string `json:"from_id"`   // Source symbol ID
	ToID     string `json:"to_id"`     // Target symbol ID
	EdgeType string `json:"edge_type"` // calls | implements | imports | references | embeds
	Weight   int    `json:"weight"`    // Relationship weight (for ranking)

	// For import edges: specific import path
	ImportPath string `json:"import_path,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
}

// Package represents a Go package with aggregated information
type Package struct {
	// Identification
	Path string `json:"path"` // Full package path
	Name string `json:"name"` // Short package name

	// Semantic role (inferred)
	Role string `json:"role"` // domain | application | infrastructure | interface | adapter | etc.

	// Generated summary
	Summary string `json:"summary"` // Package responsibilities and purpose

	// Key exports
	KeyTypes   []string `json:"key_types"`  // Most important types
	KeyFuncs   []string `json:"key_funcs"`  // Most important functions
	Interfaces []string `json:"interfaces"` // All interfaces

	// Dependencies
	Imports    []string `json:"imports"`     // What this package imports
	ImportedBy []string `json:"imported_by"` // What imports this package

	// Statistics
	FileCount   int `json:"file_count"`
	SymbolCount int `json:"symbol_count"`
	LineCount   int `json:"line_count"`

	// Metadata
	RepoPath  string    `json:"repo_path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Repository tracks indexing metadata for a repository.
type Repository struct {
	ID            string     `json:"id"`
	RootPath      string     `json:"root_path"`
	LastIndexedAt *time.Time `json:"last_indexed_at,omitempty"`
	SymbolCount   int        `json:"symbol_count"`
	PackageCount  int        `json:"package_count"`
	EdgeCount     int        `json:"edge_count"`
	HasEmbeddings bool       `json:"has_embeddings"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PackageCard is the LLM-friendly representation of a package
type PackageCard struct {
	Path       string   `json:"path"`
	Role       string   `json:"role"`
	Summary    string   `json:"summary"`
	Why        []string `json:"why"`         // Reasons for recommendation
	KeySymbols []string `json:"key_symbols"` // Important symbols to look at
	Imports    []string `json:"imports"`
	ImportedBy []string `json:"imported_by"`
}

// SymbolCard is the LLM-friendly representation of a symbol
type SymbolCard struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Signature string   `json:"signature"`
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Why       []string `json:"why"`               // Reasons for recommendation
	Snippet   string   `json:"snippet,omitempty"` // Code snippet (optional, controlled)
}

// EvidencePack is the structured context for LLM consumption
type EvidencePack struct {
	Query       string        `json:"query"`
	TopPackages []PackageCard `json:"top_packages"`
	TopSymbols  []SymbolCard  `json:"top_symbols"`
	GraphHints  []string      `json:"graph_hints"` // e.g., "handler -> service -> repo"
	Snippets    []CodeSnippet `json:"snippets"`
	Metadata    PackMetadata  `json:"metadata"`
}

// CodeSnippet represents a minimal code excerpt
type CodeSnippet struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Reason    string `json:"reason"` // Why this snippet is included
}

// PackMetadata provides information about the evidence pack
type PackMetadata struct {
	TotalSymbols    int       `json:"total_symbols"`
	TotalPackages   int       `json:"total_packages"`
	TotalLines      int       `json:"total_lines"`
	HasVectorSearch bool      `json:"has_vector_search"`
	GeneratedAt     time.Time `json:"generated_at"`
}

// Edge types constants
const (
	EdgeTypeCalls      = "calls"
	EdgeTypeImplements = "implements"
	EdgeTypeImports    = "imports"
	EdgeTypeReferences = "references"
	EdgeTypeEmbeds     = "embeds"
)

// Package role constants (inferred from directory name, imports, etc.)
const (
	RoleDomain         = "domain"
	RoleApplication    = "application"
	RoleInfrastructure = "infrastructure"
	RoleInterface      = "interface"
	RoleAdapter        = "adapter"
	RoleUtil           = "util"
	RoleUnknown        = "unknown"
)

// Symbol kind constants
const (
	KindPackage   = "package"
	KindFile      = "file"
	KindInterface = "interface"
	KindStruct    = "struct"
	KindFunc      = "func"
	KindMethod    = "method"
	KindConst     = "const"
	KindVar       = "var"
)
