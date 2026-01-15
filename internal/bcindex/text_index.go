package bcindex

import (
	"fmt"
	"os"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

type TextIndexer interface {
	IndexDoc(id string, doc TextDoc) error
	Close() error
}

type BleveIndexer struct {
	index bleve.Index
}

func CreateTextIndex(dir string) (TextIndexer, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("reset text index dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create text index dir: %w", err)
	}
	index, err := bleve.New(dir, buildIndexMapping())
	if err != nil {
		return nil, fmt.Errorf("create bleve index: %w", err)
	}
	return &BleveIndexer{index: index}, nil
}

func OpenTextIndex(dir string) (bleve.Index, error) {
	index, err := bleve.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("open bleve index: %w", err)
	}
	return index, nil
}

func (b *BleveIndexer) IndexDoc(id string, doc TextDoc) error {
	return b.index.Index(id, doc)
}

func (b *BleveIndexer) Close() error {
	return b.index.Close()
}

func buildIndexMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultAnalyzer = "en"
	indexMapping.DefaultField = "content"

	docMapping := bleve.NewDocumentMapping()

	contentField := bleve.NewTextFieldMapping()
	contentField.Store = false
	contentField.Index = true
	docMapping.AddFieldMappingsAt("content", contentField)

	pathField := bleve.NewTextFieldMapping()
	pathField.Store = true
	pathField.Index = true
	docMapping.AddFieldMappingsAt("path", pathField)

	titleField := bleve.NewTextFieldMapping()
	titleField.Store = true
	titleField.Index = true
	docMapping.AddFieldMappingsAt("title", titleField)

	kindField := bleve.NewTextFieldMapping()
	kindField.Store = true
	kindField.Index = true
	kindField.Analyzer = "keyword"
	docMapping.AddFieldMappingsAt("kind", kindField)

	lineField := bleve.NewNumericFieldMapping()
	lineField.Store = true
	lineField.Index = false
	docMapping.AddFieldMappingsAt("line_start", lineField)
	docMapping.AddFieldMappingsAt("line_end", lineField)

	indexMapping.DefaultMapping = docMapping
	return indexMapping
}
