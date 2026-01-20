package ast

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// SymbolExtractor extracts symbols from a Go package
type SymbolExtractor struct {
	pkg        *packages.Package
	repoPath   string
	symbols    []*ExtractedSymbol
	fileInfo   map[string]*fileContext
}

// ExtractedSymbol represents a symbol extracted from the AST
type ExtractedSymbol struct {
	// Basic identification
	ID          string   // Unique identifier
	Name        string   // Symbol name
	Kind        string   // package, file, struct, interface, type, func, method, const, var
	PackagePath string   // Full package path
	PackageName string   // Simple package name

	// Location
	FilePath    string   // Source file path
	LineStart   int      // Start line
	LineEnd     int      // End line

	// Declaration info
	Signature   string   // Function signature or type definition
	DocComment  string   // Documentation comment
	Exported    bool     // Is this exported?

	// Type-specific info
	Receiver    string   // For methods: receiver type
	FieldName   string   // For struct fields: field name
	FieldType   string   // For struct fields: field type

	// Additional metadata
	ParentID    string   // Parent symbol ID (for nested symbols)
	Children    []string // Child symbol IDs
	Imports     []string // Imported packages (for package-level)
}

// fileContext holds information about a file being processed
type fileContext struct {
	astFile     *ast.File
	fset        *token.FileSet
	imports     map[string]string // import path -> local alias
	pkg         *packages.Package
}

// NewSymbolExtractor creates a new symbol extractor for a package
func NewSymbolExtractor(pkg *packages.Package, repoPath string) *SymbolExtractor {
	return &SymbolExtractor{
		pkg:      pkg,
		repoPath: repoPath,
		symbols:  make([]*ExtractedSymbol, 0),
		fileInfo: make(map[string]*fileContext),
	}
}

// Extract extracts all symbols from the package
func (e *SymbolExtractor) Extract() ([]*ExtractedSymbol, error) {
	// Build file context for each syntax file
	for _, astFile := range e.pkg.Syntax {
		absFilePath := e.pkg.Fset.File(astFile.Pos()).Name()
		// Convert to relative path for worktree compatibility
		relFilePath := e.toRelPath(absFilePath)
		e.fileInfo[relFilePath] = &fileContext{
			astFile: astFile,
			fset:    e.pkg.Fset,
			pkg:     e.pkg,
			imports: e.extractImports(astFile),
		}
	}

	// Extract package-level symbol
	if err := e.extractPackageSymbol(); err != nil {
		return nil, fmt.Errorf("failed to extract package symbol: %w", err)
	}

	// Extract symbols from each file
	for filePath, ctx := range e.fileInfo {
		if err := e.extractFileSymbols(filePath, ctx); err != nil {
			return nil, fmt.Errorf("failed to extract symbols from %s: %w", filePath, err)
		}
	}

	return e.symbols, nil
}

// extractPackageSymbol creates a package-level symbol
func (e *SymbolExtractor) extractPackageSymbol() error {
	pkgID := e.packageID()

	// Gather all imports
	importSet := make(map[string]bool)
	for _, astFile := range e.pkg.Syntax {
		for _, imp := range astFile.Imports {
			path, _ := strconvQuotes(imp.Path.Value)
			importSet[path] = true
		}
	}

	var imports []string
	for imp := range importSet {
		imports = append(imports, imp)
	}

	sym := &ExtractedSymbol{
		ID:          pkgID,
		Name:        e.pkg.Name,
		Kind:        "package",
		PackagePath: e.pkg.PkgPath,
		PackageName: e.pkg.Name,
		FilePath:    e.pkg.PkgPath,
		LineStart:   1,
		LineEnd:     1,
		Signature:   fmt.Sprintf("package %s", e.pkg.Name),
		DocComment:  e.extractPackageDoc(),
		Exported:    true,
		Imports:     imports,
	}

	e.symbols = append(e.symbols, sym)
	return nil
}

// extractFileSymbols extracts all symbols from a file
func (e *SymbolExtractor) extractFileSymbols(filePath string, ctx *fileContext) error {
	// Create file-level symbol
	fileID := e.fileID(filePath)

	// Count symbols in this file for summary
	symbolCount := 0

	// Process only package-level declarations (not inside functions)
	for _, decl := range ctx.astFile.Decls {
		switch node := decl.(type) {
		case *ast.GenDecl:
			// Handle constants, variables, and type declarations
			count := e.extractGenDecl(node, filePath, ctx)
			symbolCount += count

		case *ast.FuncDecl:
			// Handle functions and methods
			sym := e.extractFuncDecl(node, filePath, ctx)
			if sym != nil {
				e.symbols = append(e.symbols, sym)
				symbolCount++
			}
		}
	}

	// Add file-level symbol
	fileSym := &ExtractedSymbol{
		ID:          fileID,
		Name:        filepath.Base(filePath),
		Kind:        "file",
		PackagePath: e.pkg.PkgPath,
		PackageName: e.pkg.Name,
		FilePath:    filePath,
		LineStart:   1,
		LineEnd:     ctx.fset.File(ctx.astFile.End()).LineCount(),
		DocComment:  fmt.Sprintf("Go source file with %d symbols", symbolCount),
		Exported:    true,
	}

	e.symbols = append(e.symbols, fileSym)
	return nil
}

// extractGenDecl extracts symbols from general declarations (const, var, type)
// Returns the number of symbols extracted
func (e *SymbolExtractor) extractGenDecl(decl *ast.GenDecl, filePath string, ctx *fileContext) int {
	count := 0
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			// Type declarations (struct, interface, type aliases)
			sym := e.extractTypeSpec(s, decl, filePath, ctx)
			if sym != nil {
				e.symbols = append(e.symbols, sym)
				count++
			}

		case *ast.ValueSpec:
			// Constants and variables
			if decl.Tok == token.CONST {
				for _, name := range s.Names {
					sym := e.extractValueSpec(name, s, decl, "const", filePath, ctx)
					if sym != nil {
						e.symbols = append(e.symbols, sym)
						count++
					}
				}
			} else if decl.Tok == token.VAR {
				for _, name := range s.Names {
					sym := e.extractValueSpec(name, s, decl, "var", filePath, ctx)
					if sym != nil {
						e.symbols = append(e.symbols, sym)
						count++
					}
				}
			}
		}
	}
	return count
}

// extractTypeSpec extracts symbols from type specifications
func (e *SymbolExtractor) extractTypeSpec(spec *ast.TypeSpec, decl *ast.GenDecl, filePath string, ctx *fileContext) *ExtractedSymbol {
	startPos := ctx.fset.Position(spec.Pos())
	endPos := ctx.fset.Position(spec.End())

	var kind string
	var signature string

	switch t := spec.Type.(type) {
	case *ast.StructType:
		kind = "struct"
		signature = e.formatStructSignature(spec, t)

	case *ast.InterfaceType:
		kind = "interface"
		signature = e.formatInterfaceSignature(spec, t)

	default:
		kind = "type"
		signature = fmt.Sprintf("type %s", spec.Name.Name)
	}

	sym := &ExtractedSymbol{
		ID:          e.symbolID(filePath, kind, spec.Name.Name),
		Name:        spec.Name.Name,
		Kind:        kind,
		PackagePath: e.pkg.PkgPath,
		PackageName: e.pkg.Name,
		FilePath:    filePath,
		LineStart:   startPos.Line,
		LineEnd:     endPos.Line,
		Signature:   signature,
		DocComment:  e.extractDocComment(decl.Doc),
		Exported:    spec.Name != nil && spec.Name.IsExported(),
		ParentID:    e.packageID(),
	}

	// Extract methods if attached to interface
	if iface, ok := spec.Type.(*ast.InterfaceType); ok {
		sym.Children = e.extractInterfaceMethods(iface, spec.Name.Name, filePath, ctx)
	}

	// Extract struct fields
	if structType, ok := spec.Type.(*ast.StructType); ok {
		sym.Children = e.extractStructFields(structType, spec.Name.Name, filePath, ctx)
	}

	return sym
}

// extractFuncDecl extracts symbols from function declarations
func (e *SymbolExtractor) extractFuncDecl(decl *ast.FuncDecl, filePath string, ctx *fileContext) *ExtractedSymbol {
	startPos := ctx.fset.Position(decl.Pos())
	endPos := ctx.fset.Position(decl.End())

	var kind string
	var receiver string

	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		kind = "func"
	} else {
		kind = "method"
		recvType := e.recvTypeToString(decl.Recv.List[0].Type)
		receiver = recvType
	}

	signature := e.formatFuncSignature(decl)

	// For methods, include receiver type in ID to ensure uniqueness
	var id string
	if kind == "method" && receiver != "" {
		id = e.symbolID(filePath, kind, fmt.Sprintf("%s.%s", receiver, decl.Name.Name))
	} else {
		id = e.symbolID(filePath, kind, decl.Name.Name)
	}

	sym := &ExtractedSymbol{
		ID:          id,
		Name:        decl.Name.Name,
		Kind:        kind,
		PackagePath: e.pkg.PkgPath,
		PackageName: e.pkg.Name,
		FilePath:    filePath,
		LineStart:   startPos.Line,
		LineEnd:     endPos.Line,
		Signature:   signature,
		DocComment:  e.extractDocComment(decl.Doc),
		Exported:    decl.Name != nil && decl.Name.IsExported(),
		Receiver:    receiver,
		ParentID:    e.packageID(),
	}

	return sym
}

// extractValueSpec extracts symbols from value specifications (const/var)
func (e *SymbolExtractor) extractValueSpec(name *ast.Ident, spec *ast.ValueSpec, decl *ast.GenDecl, kind string, filePath string, ctx *fileContext) *ExtractedSymbol {
	startPos := ctx.fset.Position(name.Pos())
	endPos := ctx.fset.Position(name.End())

	var typeStr string
	if spec.Type != nil {
		typeStr = e.typeToString(spec.Type)
	}

	sym := &ExtractedSymbol{
		ID:          e.symbolID(filePath, kind, name.Name),
		Name:        name.Name,
		Kind:        kind,
		PackagePath: e.pkg.PkgPath,
		PackageName: e.pkg.Name,
		FilePath:    filePath,
		LineStart:   startPos.Line,
		LineEnd:     endPos.Line,
		Signature:   fmt.Sprintf("%s %s %s", kind, name.Name, typeStr),
		DocComment:  e.extractDocComment(decl.Doc),
		Exported:    name.IsExported(),
		ParentID:    e.packageID(),
	}

	return sym
}

// extractInterfaceMethods extracts method symbols from an interface
func (e *SymbolExtractor) extractInterfaceMethods(iface *ast.InterfaceType, interfaceName string, filePath string, ctx *fileContext) []string {
	var children []string

	for _, method := range iface.Methods.List {
		switch m := method.Type.(type) {
		case *ast.FuncType:
			for _, name := range method.Names {
				sym := &ExtractedSymbol{
					ID:          e.symbolID(filePath, "interface-method", fmt.Sprintf("%s.%s", interfaceName, name.Name)),
					Name:        name.Name,
					Kind:        "method",
					PackagePath: e.pkg.PkgPath,
					PackageName: e.pkg.Name,
					FilePath:    filePath,
					LineStart:   ctx.fset.Position(method.Pos()).Line,
					LineEnd:     ctx.fset.Position(method.End()).Line,
					Signature:   e.formatFuncTypeSignature(name.Name, m),
					DocComment:  e.extractDocComment(method.Doc),
					Exported:    true,
					ParentID:    "", // Will be set by caller
				}
				e.symbols = append(e.symbols, sym)
				children = append(children, sym.ID)
			}
		}
	}

	return children
}

// extractStructFields extracts field symbols from a struct
func (e *SymbolExtractor) extractStructFields(structType *ast.StructType, structName string, filePath string, ctx *fileContext) []string {
	var children []string

	if structType.Fields == nil {
		return children
	}

	for i, field := range structType.Fields.List {
		for _, name := range field.Names {
			fieldType := e.typeToString(field.Type)
			fieldName := name.Name

			// Generate ID with parent struct name for uniqueness
			id := e.symbolID(filePath, "field", fmt.Sprintf("%s.%s", structName, fieldName))

			sym := &ExtractedSymbol{
				ID:          id,
				Name:        fieldName,
				Kind:        "field",
				PackagePath: e.pkg.PkgPath,
				PackageName: e.pkg.Name,
				FilePath:    filePath,
				LineStart:   ctx.fset.Position(field.Pos()).Line,
				LineEnd:     ctx.fset.Position(field.End()).Line,
				Signature:   fmt.Sprintf("%s %s", fieldName, fieldType),
				DocComment:  e.extractDocComment(field.Doc),
				Exported:    name.IsExported(),
				FieldName:   fieldName,
				FieldType:   fieldType,
			}
			e.symbols = append(e.symbols, sym)
			children = append(children, sym.ID)
		}
		// Handle anonymous/embedded fields
		if len(field.Names) == 0 {
			fieldType := e.typeToString(field.Type)
			id := e.symbolID(filePath, "field", fmt.Sprintf("%s.embedded-%d", structName, i))
			sym := &ExtractedSymbol{
				ID:          id,
				Name:        fieldType,
				Kind:        "field",
				PackagePath: e.pkg.PkgPath,
				PackageName: e.pkg.Name,
				FilePath:    filePath,
				LineStart:   ctx.fset.Position(field.Pos()).Line,
				LineEnd:     ctx.fset.Position(field.End()).Line,
				Signature:   fmt.Sprintf("embedded %s", fieldType),
				DocComment:  e.extractDocComment(field.Doc),
				Exported:    true,
				FieldName:   "",
				FieldType:   fieldType,
			}
			e.symbols = append(e.symbols, sym)
			children = append(children, sym.ID)
		}
	}

	return children
}

// Helper methods

// toRelPath converts an absolute file path to a relative path from the repository root.
// This ensures that file paths work correctly across different worktrees.
func (e *SymbolExtractor) toRelPath(absPath string) string {
	if e.repoPath == "" {
		return absPath
	}
	relPath, err := filepath.Rel(e.repoPath, absPath)
	if err != nil || filepath.IsAbs(relPath) {
		// If conversion fails or path is outside repo, return absolute path
		return absPath
	}
	// Use forward slashes for consistency
	return filepath.ToSlash(relPath)
}

func (e *SymbolExtractor) packageID() string {
	return fmt.Sprintf("pkg:%s", e.pkg.PkgPath)
}

func (e *SymbolExtractor) fileID(filePath string) string {
	return fmt.Sprintf("file:%s", filePath)
}

func (e *SymbolExtractor) symbolID(filePath, kind, name string) string {
	_ = filePath // Currently not using relative path in ID
	return fmt.Sprintf("%s:%s:%s", e.pkg.PkgPath, kind, name)
}

func (e *SymbolExtractor) extractImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, imp := range file.Imports {
		path, _ := strconvQuotes(imp.Path.Value)
		if imp.Name != nil {
			imports[path] = imp.Name.Name
		} else {
			// Extract last component of path
			parts := strings.Split(path, "/")
			imports[path] = parts[len(parts)-1]
		}
	}
	return imports
}

func (e *SymbolExtractor) extractDocComment(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	var lines []string
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

func (e *SymbolExtractor) extractPackageDoc() string {
	// Try to find package doc from the first file
	for _, file := range e.pkg.Syntax {
		if file.Doc != nil {
			return e.extractDocComment(file.Doc)
		}
	}
	return ""
}

func (e *SymbolExtractor) typeToString(typ ast.Expr) string {
	if typ == nil {
		return ""
	}

	switch t := typ.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", e.typeToString(t.X), t.Sel.Name)
	case *ast.IndexExpr:
		return fmt.Sprintf("%s[%s]", e.typeToString(t.X), e.typeToString(t.Index))
	case *ast.IndexListExpr:
		parts := make([]string, 0, len(t.Indices))
		for _, idx := range t.Indices {
			parts = append(parts, e.typeToString(idx))
		}
		return fmt.Sprintf("%s[%s]", e.typeToString(t.X), strings.Join(parts, ", "))
	case *ast.StarExpr:
		return fmt.Sprintf("*%s", e.typeToString(t.X))
	case *ast.ArrayType:
		return fmt.Sprintf("[]%s", e.typeToString(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", e.typeToString(t.Key), e.typeToString(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.StructType:
		return "struct{}"
	case *ast.ChanType:
		if t.Dir == ast.SEND {
			return fmt.Sprintf("chan<- %s", e.typeToString(t.Value))
		} else if t.Dir == ast.RECV {
			return fmt.Sprintf("<-chan %s", e.typeToString(t.Value))
		}
		return fmt.Sprintf("chan %s", e.typeToString(t.Value))
	case *ast.Ellipsis:
		return fmt.Sprintf("...%s", e.typeToString(t.Elt))
	case *ast.ParenExpr:
		return fmt.Sprintf("(%s)", e.typeToString(t.X))
	default:
		return "unknown"
	}
}

func (e *SymbolExtractor) recvTypeToString(typ ast.Expr) string {
	// Remove * if present
	if star, ok := typ.(*ast.StarExpr); ok {
		return e.typeToString(star.X)
	}
	return e.typeToString(typ)
}

func (e *SymbolExtractor) formatFuncSignature(decl *ast.FuncDecl) string {
	var builder strings.Builder

	builder.WriteString("func ")

	// Receiver
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		builder.WriteString("(")
		recv := decl.Recv.List[0]
		for i, name := range recv.Names {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(name.Name)
		}
		builder.WriteString(" ")
		builder.WriteString(e.typeToString(recv.Type))
		builder.WriteString(") ")
	}

	// Name
	builder.WriteString(decl.Name.Name)

	// Parameters
	builder.WriteString(e.formatFuncParams(decl.Type.Params))

	// Return values
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		builder.WriteString(" ")
		builder.WriteString(e.formatFuncParams(decl.Type.Results))
	}

	return builder.String()
}

func (e *SymbolExtractor) formatFuncTypeSignature(name string, ft *ast.FuncType) string {
	return fmt.Sprintf("func %s%s", name, e.formatFuncParams(ft.Params))
}

func (e *SymbolExtractor) formatFuncParams(params *ast.FieldList) string {
	if params == nil || len(params.List) == 0 {
		return "()"
	}

	var builder strings.Builder
	builder.WriteString("(")

	for i, param := range params.List {
		if i > 0 {
			builder.WriteString(", ")
		}

		// Names
		if len(param.Names) > 0 {
			for j, name := range param.Names {
				if j > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(name.Name)
			}
			builder.WriteString(" ")
		}

		// Type
		builder.WriteString(e.typeToString(param.Type))
	}

	builder.WriteString(")")
	return builder.String()
}

func (e *SymbolExtractor) formatStructSignature(spec *ast.TypeSpec, structType *ast.StructType) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("type %s struct", spec.Name))

	if structType.Fields != nil && len(structType.Fields.List) > 0 {
		builder.WriteString(" {\n")
		for _, field := range structType.Fields.List {
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					builder.WriteString(fmt.Sprintf("\t%s %s\n", name.Name, e.typeToString(field.Type)))
				}
			} else {
				builder.WriteString(fmt.Sprintf("\t%s\n", e.typeToString(field.Type)))
			}
		}
		builder.WriteString("}")
	}

	return builder.String()
}

func (e *SymbolExtractor) formatInterfaceSignature(spec *ast.TypeSpec, ifaceType *ast.InterfaceType) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("type %s interface", spec.Name))

	if ifaceType.Methods != nil && len(ifaceType.Methods.List) > 0 {
		builder.WriteString(" {\n")
		for _, method := range ifaceType.Methods.List {
			if len(method.Names) > 0 {
				if fn, ok := method.Type.(*ast.FuncType); ok {
					builder.WriteString(fmt.Sprintf("\t%s%s\n", method.Names[0].Name, e.formatFuncParams(fn.Params)))
				}
			} else {
				builder.WriteString(fmt.Sprintf("\t%s\n", e.typeToString(method.Type)))
			}
		}
		builder.WriteString("}")
	}

	return builder.String()
}

func strconvQuotes(s string) (string, bool) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1], true
	}
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		return s[1 : len(s)-1], true
	}
	return s, false
}
