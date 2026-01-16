package ast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymbolExtractor_BasicExtraction(t *testing.T) {
	// Create a temporary test package
	tmpDir := t.TempDir()

	// Create a simple Go file
	testFile := filepath.Join(tmpDir, "test.go")
	content := `
package test

import "fmt"

// Package comment
// This is a test package

// MyInt is a custom int type
type MyInt int

// String returns the string representation
func (m MyInt) String() string {
	return fmt.Sprintf("%d", m)
}

// MyStruct represents a test structure
type MyStruct struct {
	Name string
	Age  int
}

// MyInterface defines a contract
type MyInterface interface {
	DoSomething() error
	DoAnotherThing(a, b int) string
}

// MyFunc is a package-level function
func MyFunc(x int) int {
	return x * 2
}

const (
	// MaxValue is the maximum value
	MaxValue = 100
)

// GlobalVar is exported
var GlobalVar string
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Load and extract
	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	extractor := NewSymbolExtractor(pkg, tmpDir)
	symbols, err := extractor.Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Verify we got symbols
	if len(symbols) == 0 {
		t.Fatal("expected at least one symbol, got none")
	}

	// Count symbol types
	var pkgCount, typeCount, funcCount, methodCount, constCount, varCount int
	for _, sym := range symbols {
		switch sym.Kind {
		case "package":
			pkgCount++
		case "struct", "interface", "type":
			typeCount++
		case "func":
			if sym.Receiver != "" {
				methodCount++
			} else {
				funcCount++
			}
		case "method":
			methodCount++
		case "const":
			constCount++
		case "var":
			varCount++
		}
	}

	// Assertions
	if pkgCount != 1 {
		t.Errorf("expected 1 package symbol, got %d", pkgCount)
	}
	if typeCount < 3 {
		t.Errorf("expected at least 3 type symbols, got %d", typeCount)
	}
	if funcCount < 1 {
		t.Errorf("expected at least 1 function symbol, got %d", funcCount)
	}
	if methodCount < 1 {
		t.Errorf("expected at least 1 method symbol, got %d", methodCount)
	}
	if constCount < 1 {
		t.Errorf("expected at least 1 const symbol, got %d", constCount)
	}
	if varCount < 1 {
		t.Errorf("expected at least 1 var symbol, got %d", varCount)
	}
}

func TestSymbolExtractor_ExportedSymbols(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.go")
	content := `
package test

// ExportedFunc is exported
func ExportedFunc() {}

// unexportedFunc is not exported
func unexportedFunc() {}

// ExportedType is exported
type ExportedType struct {
	ExportedField   string
	unexportedField int
}

// unexportedType is not exported
type unexportedType struct {
	Field string
}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	extractor := NewSymbolExtractor(pkg, tmpDir)
	symbols, err := extractor.Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Find exported and unexported symbols
	exported := make(map[string]bool)
	unexported := make(map[string]bool)

	for _, sym := range symbols {
		if sym.Name == "" || sym.Kind == "package" || sym.Kind == "file" {
			continue
		}
		if sym.Exported {
			exported[sym.Name] = true
		} else {
			unexported[sym.Name] = true
		}
	}

	// Verify exported symbols
	if !exported["ExportedFunc"] {
		t.Error("expected ExportedFunc to be marked as exported")
	}
	if !exported["ExportedType"] {
		t.Error("expected ExportedType to be marked as exported")
	}
	if !exported["ExportedField"] {
		t.Error("expected ExportedField to be marked as exported")
	}

	// Verify unexported symbols
	if !unexported["unexportedFunc"] {
		t.Error("expected unexportedFunc to be marked as not exported")
	}
	if !unexported["unexportedType"] {
		t.Error("expected unexportedType to be marked as not exported")
	}
	if !unexported["unexportedField"] {
		t.Error("expected unexportedField to be marked as not exported")
	}
}

func TestSymbolExtractor_DocComments(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "test.go")
	content := `
package test

// DocumentedFunc has documentation
// This is a multi-line comment
func DocumentedFunc() {}

func UndocumentedFunc() {}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	loader := NewPackageLoader()
	pkg, err := loader.LoadFile(testFile)
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	extractor := NewSymbolExtractor(pkg, tmpDir)
	symbols, err := extractor.Extract()
	if err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	// Find the documented function
	var documentedFunc *ExtractedSymbol
	for _, sym := range symbols {
		if sym.Name == "DocumentedFunc" {
			documentedFunc = sym
			break
		}
	}

	if documentedFunc == nil {
		t.Fatal("could not find DocumentedFunc")
	}

	if documentedFunc.DocComment == "" {
		t.Error("expected DocumentedFunc to have documentation")
	}

	// Find the undocumented function
	var undocumentedFunc *ExtractedSymbol
	for _, sym := range symbols {
		if sym.Name == "UndocumentedFunc" {
			undocumentedFunc = sym
			break
		}
	}

	if undocumentedFunc == nil {
		t.Fatal("could not find UndocumentedFunc")
	}

	if undocumentedFunc.DocComment != "" {
		t.Error("expected UndocumentedFunc to have no documentation")
	}
}

func TestPipeline_ExtractRepository(t *testing.T) {
	// Create a temporary repository
	tmpDir := t.TempDir()

	// Create go.mod for proper module structure
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create multiple packages
	pkg1Dir := filepath.Join(tmpDir, "pkg1")
	if err := os.MkdirAll(pkg1Dir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkg1File := filepath.Join(pkg1Dir, "pkg1.go")
	if err := os.WriteFile(pkg1File, []byte(`
package pkg1

func Func1() {}
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

func Func2() {}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Extract repository
	pipeline := NewPipeline()
	symbols, err := pipeline.ExtractRepository(tmpDir)
	if err != nil {
		t.Fatalf("failed to extract repository: %v", err)
	}

	if len(symbols) == 0 {
		t.Fatal("expected at least one symbol, got none")
	}

	// Verify we have symbols from both packages
	pkg1Found := false
	pkg2Found := false

	for _, sym := range symbols {
		if sym.PackagePath == "testrepo/pkg1" {
			pkg1Found = true
		}
		if sym.PackagePath == "testrepo/pkg2" {
			pkg2Found = true
		}
	}

	if !pkg1Found {
		t.Error("expected to find symbols from pkg1")
	}
	if !pkg2Found {
		t.Error("expected to find symbols from pkg2")
	}
}

func TestPipeline_AnalyzeRepository(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod for proper module structure
	goModContent := `module testrepo

go 1.21
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create a simple package
	pkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	pkgFile := filepath.Join(pkgDir, "lib.go")
	if err := os.WriteFile(pkgFile, []byte(`
package mypkg

func ExportedFunc() {}
func internalFunc() {}
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	pipeline := NewPipeline()
	stats, err := pipeline.AnalyzeRepository(tmpDir)
	if err != nil {
		t.Fatalf("failed to analyze repository: %v", err)
	}

	if stats.PackageCount != 1 {
		t.Errorf("expected 1 package, got %d", stats.PackageCount)
	}
	if stats.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", stats.FileCount)
	}
	if stats.SymbolCount == 0 {
		t.Error("expected at least 1 symbol, got 0")
	}
	if stats.ExportedCount == 0 {
		t.Error("expected at least 1 exported symbol, got 0")
	}
}
