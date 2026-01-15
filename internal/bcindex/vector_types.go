package bcindex

type VectorPoint struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

type EmbeddingResult struct {
	Index  int
	Vector []float32
}

type VectorChunk struct {
	ID        string
	File      string
	Kind      string
	Name      string
	Title     string
	Text      string
	LineStart int
	LineEnd   int
	Hash      string
}
