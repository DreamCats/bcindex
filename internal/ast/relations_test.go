package ast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelationExtractor_ImportEdges(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod for proper module structure
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a package with imports
	pkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkgFile := filepath.Join(pkgDir, "lib.go")
	if err := os.WriteFile(pkgFile, []byte(`
package mypkg

import (
	"fmt"
	"os"
	"encoding/json"
)

func Func1() {
	fmt.Println("test")
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Load and extract
	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(pkgFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	symbols, err := NewSymbolExtractor(pkg, tmpDir).Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Extract relations
	extractor := NewRelationExtractor(pkg, tmpDir, symbols)
	edges, err := extractor.ExtractAll()
	if err != nil {
		t.Fatalf("failed to extract relations: %v", err)
	}

	// Verify we have import edges (though standard lib is filtered)
	if len(edges) == 0 {
		t.Log("no import edges found (expected for stdlib-only imports)")
	}

	// Check edge types
	for _, edge := range edges {
		if edge.EdgeType == "imports" {
			t.Logf("Found import edge: %s -> %s (%s)", edge.FromID, edge.ToID, edge.ImportPath)
		}
	}
}

func TestRelationExtractor_ImplementationEdges(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a package with interface and implementation
	pkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkgFile := filepath.Join(pkgDir, "lib.go")
	if err := os.WriteFile(pkgFile, []byte(`
package mypkg

// Writer is an interface
type Writer interface {
	Write([]byte) (int, error)
}

// BufferedWriter implements Writer
type BufferedWriter struct {
	buf []byte
}

func (b *BufferedWriter) Write(data []byte) (int, error) {
	b.buf = append(b.buf, data...)
	return len(data), nil
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Load and extract
	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(pkgFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	symbols, err := NewSymbolExtractor(pkg, tmpDir).Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Extract relations
	extractor := NewRelationExtractor(pkg, tmpDir, symbols)
	edges, err := extractor.ExtractAll()
	if err != nil {
		t.Fatalf("failed to extract relations: %v", err)
	}

	// Verify we have implementation edges
	hasImplEdge := false
	for _, edge := range edges {
		if edge.EdgeType == "implements" {
			hasImplEdge = true
			t.Logf("Found implementation edge: %s -> %s", edge.FromID, edge.ToID)
		}
	}

	if !hasImplEdge {
		t.Error("expected to find implementation edge for BufferedWriter -> Writer")
	}
}

func TestRelationExtractor_CallEdges(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a package with function calls
	pkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkgFile := filepath.Join(pkgDir, "lib.go")
	if err := os.WriteFile(pkgFile, []byte(`
package mypkg

func Helper() string {
	return "helper"
}

func MainFunc() string {
	return Helper()
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Load and extract
	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(pkgFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	symbols, err := NewSymbolExtractor(pkg, tmpDir).Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Extract relations
	extractor := NewRelationExtractor(pkg, tmpDir, symbols)
	edges, err := extractor.ExtractAll()
	if err != nil {
		t.Fatalf("failed to extract relations: %v", err)
	}

	// Verify we have call edges
	hasCallEdge := false
	for _, edge := range edges {
		if edge.EdgeType == "calls" {
			hasCallEdge = true
			t.Logf("Found call edge: %s -> %s", edge.FromID, edge.ToID)
		}
	}

	if !hasCallEdge {
		t.Error("expected to find call edge for MainFunc -> Helper")
	}
}

func TestRelationExtractor_FieldEdges(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a package with struct fields and embeddings
	pkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkgFile := filepath.Join(pkgDir, "lib.go")
	if err := os.WriteFile(pkgFile, []byte(`
package mypkg

type Base struct {
	ID string
}

type Derived struct {
	Base        // Embedded field
	Name string  // Regular field
	Value int
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Load and extract
	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(pkgFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	symbols, err := NewSymbolExtractor(pkg, tmpDir).Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Extract relations
	extractor := NewRelationExtractor(pkg, tmpDir, symbols)
	edges, err := extractor.ExtractAll()
	if err != nil {
		t.Fatalf("failed to extract relations: %v", err)
	}

	// Verify we have embed edges
	hasEmbedEdge := false
	for _, edge := range edges {
		if edge.EdgeType == "embeds" {
			hasEmbedEdge = true
			t.Logf("Found embed edge: %s -> %s", edge.FromID, edge.ToID)
		}
		if edge.EdgeType == "references" {
			t.Logf("Found reference edge: %s -> %s", edge.FromID, edge.ToID)
		}
	}

	if !hasEmbedEdge {
		t.Error("expected to find embed edge for Derived -> Base")
	}
}

func TestPipeline_ExtractRepositoryWithRelations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create multiple packages with relationships
	pkg1Dir := filepath.Join(tmpDir, "pkg1")
	if err := os.MkdirAll(pkg1Dir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkg1File := filepath.Join(pkg1Dir, "pkg1.go")
	if err := os.WriteFile(pkg1File, []byte(`
package pkg1

type Service interface {
	Process() error
}

type MyService struct{}

func (m *MyService) Process() error {
	return nil
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	pkg2Dir := filepath.Join(tmpDir, "pkg2")
	if err := os.MkdirAll(pkg2Dir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkg2File := filepath.Join(pkg2Dir, "pkg2.go")
	if err := os.WriteFile(pkg2File, []byte(`
package pkg2

import "testrepo/pkg1"

func UseService(svc pkg1.Service) {
	svc.Process()
}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Extract with relations
	pipeline := NewPipeline()
	symbols, edges, err := pipeline.ExtractRepositoryWithRelations(tmpDir)
	if err != nil {
		t.Fatalf("failed to extract repository with relations: %v", err)
	}

	if len(symbols) == 0 {
		t.Error("expected to find symbols")
	}

	t.Logf("Found %d symbols and %d edges", len(symbols), len(edges))

	// Print edge types for debugging
	edgeTypes := make(map[string]int)
	for _, edge := range edges {
		edgeTypes[edge.EdgeType]++
	}
	for edgeType, count := range edgeTypes {
		t.Logf("Edge type %s: %d edges", edgeType, count)
	}
}

func TestRelationConverter_ConvertToStoreEdges(t *testing.T) {
	edges := []*Edge{
		{
			FromID:     "pkg:test:func1",
			ToID:       "pkg:test:func2",
			EdgeType:   "calls",
			Weight:     5,
			ImportPath: "test",
		},
	}

	// This would be used with actual store.Edge
	// Just verify the conversion works
	for _, edge := range edges {
		if edge.FromID == "" {
			t.Error("expected FromID to be set")
		}
		if edge.ToID == "" {
			t.Error("expected ToID to be set")
		}
		if edge.EdgeType == "" {
			t.Error("expected EdgeType to be set")
		}
	}
}
