package ast

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackageLoader loads Go packages from a repository root
type PackageLoader struct {
	// Mode specifies the package loading mode
	Mode packages.LoadMode

	// Overlay maps file paths to alternative content
	Overlay map[string][]byte

	// BuildTags specifies build tags to respect
	BuildTags []string

	// Tests indicates whether to load test files
	Tests bool

	// SkipDir specifies directories to skip
	SkipDir []string
}

// NewPackageLoader creates a new package loader with sensible defaults
func NewPackageLoader() *PackageLoader {
	return &PackageLoader{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedModule,
		Tests:   false,
		SkipDir: []string{"vendor", "third_party", ".git"},
	}
}

// LoadConfig holds configuration for loading a repository
type LoadConfig struct {
	// Root is the repository root path
	Root string

	// Patterns specifies which packages to load (default: ["./..."])
	Patterns []string

	// Tests indicates whether to include test packages
	Tests bool

	// BuildTags specifies build tags to respect
	BuildTags []string
}

// LoadRepo loads all packages from a repository root
func (l *PackageLoader) LoadRepo(config *LoadConfig) ([]*packages.Package, error) {
	if config == nil {
		config = &LoadConfig{}
	}

	// Validate root path
	if config.Root == "" {
		return nil, fmt.Errorf("root path is required")
	}

	// Check if root exists
	if _, err := os.Stat(config.Root); os.IsNotExist(err) {
		return nil, fmt.Errorf("root path does not exist: %s", config.Root)
	}

	// Resolve absolute path
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}

	// Default patterns
	patterns := config.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	// Convert patterns to absolute paths
	for i, p := range patterns {
		if strings.HasPrefix(p, "./") || strings.HasPrefix(p, ".\\") {
			patterns[i] = filepath.Join(root, strings.TrimPrefix(p, "./"))
		}
	}

	// Build load config
	cfg := &packages.Config{
		Dir:         root,
		Mode:        l.Mode,
		Tests:       config.Tests || l.Tests,
		Overlay:     l.Overlay,
		BuildFlags:  l.buildFlags(config.BuildTags),
		Logf:        func(format string, args ...interface{}) {}, // Suppress noisy logs
	}

	// Load packages
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Filter out packages that failed to load or are in skipped directories
	var result []*packages.Package
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}

		// Skip if in a skipped directory
		if l.shouldSkip(pkg.PkgPath) {
			continue
		}

		// Skip if there were errors loading this package
		if len(pkg.Errors) > 0 {
			// Log but don't fail - continue with partial results
			continue
		}

		result = append(result, pkg)
	}

	return result, nil
}

// LoadPackage loads a single package by its import path
func (l *PackageLoader) LoadPackage(importPath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode:       l.Mode,
		Tests:      l.Tests,
		Overlay:    l.Overlay,
		BuildFlags: l.buildFlags(nil),
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %s: %w", importPath, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for %s", importPath)
	}

	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("errors loading package %s: %v", importPath, pkg.Errors)
	}

	return pkg, nil
}

// LoadFile loads a single Go file as a package
func (l *PackageLoader) LoadFile(filePath string) (*packages.Package, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}

	dir := filepath.Dir(absPath)
	filename := filepath.Base(absPath)

	cfg := &packages.Config{
		Mode:       l.Mode,
		Dir:        dir,
		Tests:      l.Tests,
		Overlay:    l.Overlay,
		BuildFlags: l.buildFlags(nil),
	}

	pkgs, err := packages.Load(cfg, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s: %w", filePath, err)
	}

	if len(pkgs) == 0 || pkgs[0] == nil {
		return nil, fmt.Errorf("no package found for file %s", filePath)
	}

	return pkgs[0], nil
}

// buildFlags constructs build flags for the package loader
func (l *PackageLoader) buildFlags(tags []string) []string {
	var flags []string

	// Add custom build tags
	allTags := append([]string{}, l.BuildTags...)
	allTags = append(allTags, tags...)

	for _, tag := range allTags {
		flags = append(flags, "-tags", tag)
	}

	return flags
}

// shouldSkip checks if a package path should be skipped
func (l *PackageLoader) shouldSkip(pkgPath string) bool {
	for _, skip := range l.SkipDir {
		if strings.Contains(pkgPath, skip) {
			return true
		}
	}

	// Skip generated files and vendor
	if strings.Contains(pkgPath, "vendor") ||
		strings.Contains(pkgPath, "third_party") {
		return true
	}

	return false
}

// ParseFile parses a single Go file into an AST
func (l *PackageLoader) ParseFile(filePath string) (*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()

	// Parse file with comments
	file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
	}

	return file, fset, nil
}

// FindGoFiles finds all Go files in a directory recursively
func (l *PackageLoader) FindGoFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip hidden directories and common skip directories
			base := filepath.Base(path)
			if base == "." || strings.HasPrefix(base, ".") {
				if path != root {
					return filepath.SkipDir
				}
			}

			for _, skip := range l.SkipDir {
				if base == skip {
					return filepath.SkipDir
				}
			}

			return nil
		}

		// Only include .go files (exclude test files if Tests is false)
		if strings.HasSuffix(path, ".go") {
			if !l.Tests && strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}
