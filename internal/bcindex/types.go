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

type TextDoc struct {
	Path      string `json:"path" bleve:"path"`
	Kind      string `json:"kind" bleve:"kind"`
	Title     string `json:"title" bleve:"title"`
	Content   string `json:"content" bleve:"content"`
	LineStart int    `json:"line_start" bleve:"line_start"`
	LineEnd   int    `json:"line_end" bleve:"line_end"`
}

type SearchHit struct {
	Kind    string
	Name    string
	File    string
	Line    int
	Score   float64
	Snippet string
}

type Status struct {
	RepoID      string
	Root        string
	LastIndexAt time.Time
	Symbols     int
	TextDocs    uint64
}
