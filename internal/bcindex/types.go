package bcindex

import "time"

type RepoMeta struct {
	RepoID      string    `json:"repo_id"`
	Root        string    `json:"root"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastIndexAt time.Time `json:"last_index_at"`
}

type RepoPaths struct {
	RepoID    string
	Root      string
	BaseDir   string
	RepoDir   string
	TextDir   string
	SymbolDir string
	MetaDir   string
	MetaFile  string
}

type Symbol struct {
	Name string
	Kind string
	File string
	Line int
	Pkg  string
	Recv string
	Doc  string
}

const (
	RelationKindImports   = "imports"
	RelationKindDependsOn = "depends_on"
	RelationSourceAST     = "ast"
	RelationSourceGoList  = "go_list"
)

type Relation struct {
	FromRef    string
	ToRef      string
	Kind       string
	File       string
	Line       int
	Source     string
	Confidence float64
}

const (
	DocLinkSourceMarkdown = "md_backtick"
)

type DocLink struct {
	Symbol     string
	Line       int
	Source     string
	Confidence float64
}

type RelationEdge struct {
	FromRef    string  `json:"from_ref"`
	ToRef      string  `json:"to_ref"`
	Line       int     `json:"line"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type RelationSummary struct {
	Kind  string         `json:"kind"`
	Edges []RelationEdge `json:"edges,omitempty"`
}

type DocLinkHit struct {
	Symbol     string  `json:"symbol"`
	Line       int     `json:"line"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
}

type RelationPairStat struct {
	FromRef string `json:"from_ref"`
	ToRef   string `json:"to_ref"`
	Count   int    `json:"count"`
}

type IndexContext struct {
	Tier            IndexTier
	DirImportPath   map[string]string
}

type TextDoc struct {
	Path      string `json:"path" bleve:"path"`
	Kind      string `json:"kind" bleve:"kind"`
	Title     string `json:"title" bleve:"title"`
	Content   string `json:"content" bleve:"content"`
	LineStart int    `json:"line_start" bleve:"line_start"`
	LineEnd   int    `json:"line_end" bleve:"line_end"`
}

type SearchHit struct {
	Kind       string            `json:"kind"`
	Source     string            `json:"source"`
	Name       string            `json:"name,omitempty"`
	File       string            `json:"file"`
	Line       int               `json:"line"`
	LineEnd    int               `json:"line_end,omitempty"`
	Score      float64           `json:"score"`
	Snippet    string            `json:"snippet,omitempty"`
	Relations  []RelationSummary `json:"relations,omitempty"`
	DocLinks   []DocLinkHit      `json:"doc_links,omitempty"`
	Truncated  bool              `json:"truncated,omitempty"`
	Truncation string            `json:"truncation,omitempty"`
}

type Status struct {
	RepoID      string
	Root        string
	LastIndexAt time.Time
	Symbols     int
	TextDocs    uint64
}
