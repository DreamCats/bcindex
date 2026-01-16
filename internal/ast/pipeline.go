package ast

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// Pipeline provides a high-level interface for loading and extracting symbols
type Pipeline struct {
	loader *PackageLoader
}

// NewPipeline creates a new extraction pipeline
func NewPipeline() *Pipeline {
	return &Pipeline{
		loader: NewPackageLoader(),
	}
}

// ExtractRepository extracts all symbols from a repository
func (p *Pipeline) ExtractRepository(root string) ([]*ExtractedSymbol, error) {
	// Get absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	// Check if root exists
	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		return nil, fmt.Errorf("repository root does not exist: %s", absRoot)
	}

	// Load all packages
	config := &LoadConfig{
		Root:   absRoot,
		Patterns: []string{"./..."},
		Tests:  false,
	}

	pkgs, err := p.loader.LoadRepo(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in repository")
	}

	// Extract symbols from each package
	var allSymbols []*ExtractedSymbol
	for _, pkg := range pkgs {
		symbols, err := p.ExtractPackage(pkg, absRoot)
		if err != nil {
			// Log error but continue with other packages
			continue
		}
		allSymbols = append(allSymbols, symbols...)
	}

	return allSymbols, nil
}

// ExtractPackage extracts symbols from a single package
func (p *Pipeline) ExtractPackage(pkg *packages.Package, repoPath string) ([]*ExtractedSymbol, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package is nil")
	}

	// Skip packages with errors
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package has errors: %v", pkg.Errors)
	}

	// Skip vendor packages
	for _, skip := range p.loader.SkipDir {
		if filepath.HasPrefix(pkg.PkgPath, skip) {
			return nil, fmt.Errorf("skipping package: %s", pkg.PkgPath)
		}
	}

	extractor := NewSymbolExtractor(pkg, repoPath)
	symbols, err := extractor.Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to extract symbols: %w", err)
	}

	return symbols, nil
}

// ExtractPackageByPath loads and extracts a package by its import path
func (p *Pipeline) ExtractPackageByPath(importPath string, repoPath string) ([]*ExtractedSymbol, error) {
	pkg, err := p.loader.LoadPackage(importPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %s: %w", importPath, err)
	}

	return p.ExtractPackage(pkg, repoPath)
}

// ExtractFile loads and extracts symbols from a single file
func (p *Pipeline) ExtractFile(filePath string, repoPath string) ([]*ExtractedSymbol, error) {
	pkg, err := p.loader.LoadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s: %w", filePath, err)
	}

	return p.ExtractPackage(pkg, repoPath)
}

// RepositoryStats holds statistics about a repository
type RepositoryStats struct {
	PackageCount  int
	SymbolCount   int
	FileCount     int
	ExportedCount int
	Errors        []string
}

// AnalyzeRepository analyzes a repository and returns statistics
func (p *Pipeline) AnalyzeRepository(root string) (*RepositoryStats, error) {
	stats := &RepositoryStats{
		Errors: make([]string, 0),
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root: %w", err)
	}

	config := &LoadConfig{
		Root:     absRoot,
		Patterns: []string{"./..."},
		Tests:    false,
	}

	pkgs, err := p.loader.LoadRepo(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository: %w", err)
	}

	seenFiles := make(map[string]bool)

	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}

		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				stats.Errors = append(stats.Errors, e.Error())
			}
			continue
		}

		stats.PackageCount++

		// Track files
		for _, file := range pkg.GoFiles {
			if !seenFiles[file] {
				seenFiles[file] = true
				stats.FileCount++
			}
		}

		// Extract and count symbols
		symbols, err := p.ExtractPackage(pkg, absRoot)
		if err != nil {
			stats.Errors = append(stats.Errors, err.Error())
			continue
		}

		stats.SymbolCount += len(symbols)

		for _, sym := range symbols {
			if sym.Exported {
				stats.ExportedCount++
			}
		}
	}

	return stats, nil
}

// SetBuildTags sets build tags for the loader
func (p *Pipeline) SetBuildTags(tags []string) {
	p.loader.BuildTags = tags
}

// SetTests sets whether to include test files
func (p *Pipeline) SetTests(include bool) {
	p.loader.Tests = include
}

// SetSkipDir sets directories to skip
func (p *Pipeline) SetSkipDir(dirs []string) {
	p.loader.SkipDir = dirs
}

// ExtractRepositoryWithRelations extracts both symbols and relationships
func (p *Pipeline) ExtractRepositoryWithRelations(root string) ([]*ExtractedSymbol, []*Edge, error) {
	// Get absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	// Check if root exists
	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("repository root does not exist: %s", absRoot)
	}

	// Load all packages
	config := &LoadConfig{
		Root:     absRoot,
		Patterns: []string{"./..."},
		Tests:    false,
	}

	pkgs, err := p.loader.LoadRepo(config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load repository: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages found in repository")
	}

	// Extract symbols from each package
	var allSymbols []*ExtractedSymbol
	var allEdges []*Edge

	for _, pkg := range pkgs {
		symbols, err := p.ExtractPackage(pkg, absRoot)
		if err != nil {
			// Log error but continue with other packages
			continue
		}
		allSymbols = append(allSymbols, symbols...)

		// Extract relationships for this package
		edges, err := p.ExtractRelations(pkg, absRoot, symbols)
		if err != nil {
			// Log error but continue
			continue
		}
		allEdges = append(allEdges, edges...)
	}

	return allSymbols, allEdges, nil
}

// ExtractRelations extracts relationships from a package
func (p *Pipeline) ExtractRelations(pkg *packages.Package, repoPath string, symbols []*ExtractedSymbol) ([]*Edge, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package is nil")
	}

	// Skip packages with errors
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package has errors: %v", pkg.Errors)
	}

	// Skip vendor packages
	for _, skip := range p.loader.SkipDir {
		if filepath.HasPrefix(pkg.PkgPath, skip) {
			return nil, fmt.Errorf("skipping package: %s", pkg.PkgPath)
		}
	}

	extractor := NewRelationExtractor(pkg, repoPath, symbols)
	edges, err := extractor.ExtractAll()
	if err != nil {
		return nil, fmt.Errorf("failed to extract relations: %w", err)
	}

	return edges, nil
}
