package docgen

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Scanner scans Go source files for symbols missing documentation
type Scanner struct {
	repoPath   string
	include    []string
	exclude    []string
	skipTests  bool
	maxPerFile int
	maxTotal   int
}

// ScanResult represents a symbol that needs documentation
type ScanResult struct {
	File        string
	Package     string
	SymbolName  string
	SymbolKind  string // func, method, type, struct, interface
	Signature   string
	StartLine   int
	EndLine     int
	ExistingDoc string
	Receiver    string // for methods
}

// NewScanner creates a new scanner
func NewScanner(repoPath string, opts ...Option) *Scanner {
	s := &Scanner{
		repoPath:   repoPath,
		skipTests:  true,
		maxPerFile: 50,
		maxTotal:   200,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures the scanner
type Option func(*Scanner)

// WithInclude sets include patterns
func WithInclude(patterns ...string) Option {
	return func(s *Scanner) {
		s.include = patterns
	}
}

// WithExclude sets exclude patterns
func WithExclude(patterns ...string) Option {
	return func(s *Scanner) {
		s.exclude = patterns
	}
}

// WithSkipTests sets whether to skip test files
func WithSkipTests(skip bool) Option {
	return func(s *Scanner) {
		s.skipTests = skip
	}
}

// WithMaxPerFile sets max symbols per file
func WithMaxPerFile(max int) Option {
	return func(s *Scanner) {
		s.maxPerFile = max
	}
}

// WithMaxTotal sets max total symbols
func WithMaxTotal(max int) Option {
	return func(s *Scanner) {
		s.maxTotal = max
	}
}

// Scan scans the repository for symbols missing documentation
func (s *Scanner) Scan(ctx context.Context) ([]ScanResult, error) {
	var results []ScanResult
	var mu sync.Mutex

	// Walk through the directory
	err := filepath.Walk(s.repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check context for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories
		if info.IsDir() {
			// Skip vendor, hidden dirs, etc.
			if s.shouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-Go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files if configured
		if s.skipTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Check include/exclude patterns
		relPath, _ := filepath.Rel(s.repoPath, path)
		if !s.matchesPatterns(relPath) {
			return nil
		}

		// Check max limit
		mu.Lock()
		if s.maxTotal > 0 && len(results) >= s.maxTotal {
			mu.Unlock()
			return fmt.Errorf("reached max total symbols limit")
		}
		mu.Unlock()

		// Scan the file
		fileResults, err := s.scanFile(path)
		if err != nil {
			// Log error but continue scanning
			fmt.Fprintf(os.Stderr, "Warning: failed to scan %s: %v\n", path, err)
			return nil
		}

		mu.Lock()
		// Limit per file
		if s.maxPerFile > 0 && len(fileResults) > s.maxPerFile {
			fileResults = fileResults[:s.maxPerFile]
		}
		// Limit total
		remaining := s.maxTotal - len(results)
		if s.maxTotal > 0 && len(fileResults) > remaining {
			fileResults = fileResults[:remaining]
		}
		results = append(results, fileResults...)
		mu.Unlock()

		return nil
	})

	return results, err
}

// shouldSkipDir checks if a directory should be skipped
func (s *Scanner) shouldSkipDir(path string) bool {
	base := filepath.Base(path)
	// Skip common directories to ignore
	skipDirs := map[string]bool{
		"vendor":       true,
		"node_modules": true,
		".git":         true,
		".idea":        true,
		"testdata":     true,
		"third_party":  true,
	}
	return skipDirs[base] || strings.HasPrefix(base, ".") && base != "."
}

// matchesPatterns checks if a file matches include/exclude patterns
func (s *Scanner) matchesPatterns(relPath string) bool {
	// Check exclude patterns
	for _, pattern := range s.exclude {
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return false
		}
		// Also check if path starts with pattern (for directory patterns)
		if strings.HasPrefix(relPath, pattern) {
			return false
		}
	}

	// If no include patterns, include everything not excluded
	if len(s.include) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range s.include {
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}
		// Also check if path starts with pattern
		if strings.HasPrefix(relPath, pattern) {
			return true
		}
	}

	return false
}

// scanFile scans a single Go file for symbols missing documentation
func (s *Scanner) scanFile(filePath string) ([]ScanResult, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var results []ScanResult
	pkgName := node.Name.Name

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			result := s.scanFuncDecl(d, fset, filePath, pkgName)
			if result != nil {
				results = append(results, *result)
			}

		case *ast.GenDecl:
			declResults := s.scanGenDecl(d, fset, filePath, pkgName)
			results = append(results, declResults...)
		}
	}

	return results, nil
}

// scanFuncDecl scans a function declaration
func (s *Scanner) scanFuncDecl(decl *ast.FuncDecl, fset *token.FileSet, filePath, pkgName string) *ScanResult {
	// Skip if has doc comment
	if decl.Doc != nil && len(decl.Doc.List) > 0 {
		return nil
	}

	pos := fset.Position(decl.Pos())
	end := fset.Position(decl.End())

	kind := "func"
	receiver := ""
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = "method"
		receiver = s.recvTypeToString(decl.Recv.List[0].Type)
	}

	return &ScanResult{
		File:        filePath,
		Package:     pkgName,
		SymbolName:  decl.Name.Name,
		SymbolKind:  kind,
		Signature:   s.formatFuncSignature(decl, receiver),
		StartLine:   pos.Line,
		EndLine:     end.Line,
		ExistingDoc: "",
		Receiver:    receiver,
	}
}

// scanGenDecl scans a general declaration (type only)
func (s *Scanner) scanGenDecl(decl *ast.GenDecl, fset *token.FileSet, filePath, pkgName string) []ScanResult {
	var results []ScanResult

	// Skip const and var declarations
	if decl.Tok == token.CONST || decl.Tok == token.VAR {
		return results
	}

	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.TypeSpec:
			// Check for type declarations (struct, interface, type alias)
			if spec.Doc == nil || len(spec.Doc.List) == 0 {
				result := s.scanTypeSpec(spec, decl, fset, filePath, pkgName)
				if result != nil {
					results = append(results, *result)
				}
			}
		}
	}

	return results
}

// scanTypeSpec scans a type specification
func (s *Scanner) scanTypeSpec(spec *ast.TypeSpec, decl *ast.GenDecl, fset *token.FileSet, filePath, pkgName string) *ScanResult {
	pos := fset.Position(spec.Pos())
	end := fset.Position(spec.End())

	kind := "type"
	switch spec.Type.(type) {
	case *ast.StructType:
		kind = "struct"
	case *ast.InterfaceType:
		kind = "interface"
	}

	return &ScanResult{
		File:        filePath,
		Package:     pkgName,
		SymbolName:  spec.Name.Name,
		SymbolKind:  kind,
		Signature:   fmt.Sprintf("type %s", spec.Name.Name),
		StartLine:   pos.Line,
		EndLine:     end.Line,
		ExistingDoc: "",
	}
}

// recvTypeToString converts a receiver type to string
func (s *Scanner) recvTypeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return s.recvTypeToString(t.X)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[]", s.recvTypeToString(t.X))
	case *ast.IndexListExpr:
		// Handle type parameters like [T any]
		return fmt.Sprintf("%s[...]", s.recvTypeToString(t.X))
	default:
		return ""
	}
}

// formatFuncSignature formats a function signature
func (s *Scanner) formatFuncSignature(decl *ast.FuncDecl, receiver string) string {
	var b strings.Builder
	b.WriteString("func ")
	if receiver != "" {
		b.WriteString("(")
		b.WriteString(receiver)
		b.WriteString(") ")
	}
	b.WriteString(decl.Name.Name)
	b.WriteString(s.formatParams(decl.Type.Params))
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		b.WriteString(" ")
		b.WriteString(s.formatParams(decl.Type.Results))
	}
	return b.String()
}

// formatParams formats function parameters
func (s *Scanner) formatParams(params *ast.FieldList) string {
	if params == nil || len(params.List) == 0 {
		return "()"
	}

	var b strings.Builder
	b.WriteString("(")
	for i, param := range params.List {
		if i > 0 {
			b.WriteString(", ")
		}
		if len(param.Names) > 0 {
			for j, name := range param.Names {
				if j > 0 {
					b.WriteString(", ")
				}
				b.WriteString(name.Name)
			}
			b.WriteString(" ")
		}
		b.WriteString(s.typeToString(param.Type))
	}
	b.WriteString(")")
	return b.String()
}

// typeToString converts a type expression to string
func (s *Scanner) typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", s.typeToString(t.X), t.Sel.Name)
	case *ast.StarExpr:
		return "*" + s.typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + s.typeToString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", s.typeToString(t.Key), s.typeToString(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.StructType:
		return "struct{}"
	case *ast.ChanType:
		if t.Dir == ast.SEND {
			return fmt.Sprintf("chan<- %s", s.typeToString(t.Value))
		} else if t.Dir == ast.RECV {
			return fmt.Sprintf("<-chan %s", s.typeToString(t.Value))
		}
		return fmt.Sprintf("chan %s", s.typeToString(t.Value))
	case *ast.Ellipsis:
		return "..." + s.typeToString(t.Elt)
	case *ast.ParenExpr:
		return "(" + s.typeToString(t.X) + ")"
	default:
		return "any"
	}
}
