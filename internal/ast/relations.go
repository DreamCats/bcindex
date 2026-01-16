package ast

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// RelationExtractor extracts relationships between symbols
type RelationExtractor struct {
	pkg        *packages.Package
	repoPath   string
	symbols    map[string]*ExtractedSymbol // ID -> Symbol
	edges      []*Edge
}

// Edge represents a relationship between two symbols
type Edge struct {
	FromID     string // Source symbol ID
	ToID       string // Target symbol ID
	EdgeType   string // imports | implements | calls | references | embeds
	Weight     int    // Relationship weight (for ranking)
	ImportPath string // For import edges: specific import path
}

// NewRelationExtractor creates a new relation extractor
func NewRelationExtractor(pkg *packages.Package, repoPath string, symbols []*ExtractedSymbol) *RelationExtractor {
	symMap := make(map[string]*ExtractedSymbol)
	for _, sym := range symbols {
		symMap[sym.ID] = sym
	}

	return &RelationExtractor{
		pkg:      pkg,
		repoPath: repoPath,
		symbols:  symMap,
		edges:    make([]*Edge, 0),
	}
}

// ExtractAll extracts all types of relationships
func (r *RelationExtractor) ExtractAll() ([]*Edge, error) {
	// Extract import relationships
	if err := r.extractImportEdges(); err != nil {
		return nil, fmt.Errorf("failed to extract import edges: %w", err)
	}

	// Extract implementation relationships
	if err := r.extractImplementationEdges(); err != nil {
		return nil, fmt.Errorf("failed to extract implementation edges: %w", err)
	}

	// Extract call relationships
	if err := r.extractCallEdges(); err != nil {
		return nil, fmt.Errorf("failed to extract call edges: %w", err)
	}

	// Extract field/embedding relationships
	if err := r.extractFieldEdges(); err != nil {
		return nil, fmt.Errorf("failed to extract field edges: %w", err)
	}

	return r.edges, nil
}

// extractImportEdges extracts package import relationships
func (r *RelationExtractor) extractImportEdges() error {
	if r.pkg.Types == nil || r.pkg.TypesInfo == nil {
		return nil
	}

	// Build a map of import paths to package IDs
	importPathToPkgID := make(map[string]string)
	for _, sym := range r.symbols {
		if sym.Kind == "package" {
			importPathToPkgID[sym.PackagePath] = sym.ID
		}
	}

	// Extract imports from each file
	for _, astFile := range r.pkg.Syntax {
		for _, imp := range astFile.Imports {
			importPath, _ := strconvQuotes(imp.Path.Value)

			// Skip standard library and vendor
			if r.isStdLib(importPath) || strings.Contains(importPath, "/vendor/") {
				continue
			}

			// Find the package symbol for this file
			fromPkgID := fmt.Sprintf("pkg:%s", r.pkg.PkgPath)
			toPkgID, ok := importPathToPkgID[importPath]
			if !ok {
				// Skip external packages (not indexed in our database)
				// This prevents foreign key constraint errors
				continue
			}

			edge := &Edge{
				FromID:     fromPkgID,
				ToID:       toPkgID,
				EdgeType:   "imports",
				Weight:     1,
				ImportPath: importPath,
			}

			r.edges = append(r.edges, edge)
		}
	}

	return nil
}

// extractImplementationEdges extracts interface implementation relationships
func (r *RelationExtractor) extractImplementationEdges() error {
	if r.pkg.Types == nil || r.pkg.TypesInfo == nil {
		return nil
	}

	info := r.pkg.TypesInfo

	// Find all interface types by scanning the AST
	interfaces := make(map[string]*types.Interface)
	for _, astFile := range r.pkg.Syntax {
		ast.Inspect(astFile, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			// Get the type object
			obj := info.ObjectOf(typeSpec.Name)
			if obj == nil {
				return true
			}

			// Check if it's an interface
			ifaceType, ok := obj.Type().Underlying().(*types.Interface)
			if ok {
				interfaces[obj.Id()] = ifaceType
			}

			return true
		})
	}

	// Check each named type for interface implementations
	for _, astFile := range r.pkg.Syntax {
		ast.Inspect(astFile, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			// Get the type object
			obj := info.ObjectOf(typeSpec.Name)
			if obj == nil || obj.Type() == nil {
				return true
			}

			named, ok := obj.Type().(*types.Named)
			if !ok {
				return true
			}

			// Check if this type implements any interfaces
			for ifaceID, iface := range interfaces {
				// Skip self
				if obj.Id() == ifaceID {
					continue
				}

				if types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface) {
					// Create edge from concrete type to interface
					fromSym := r.findSymbolByObj(obj)
					toSym := r.findSymbolByTypesID(ifaceID)

					if fromSym != nil && toSym != nil {
						edge := &Edge{
							FromID:   fromSym.ID,
							ToID:     toSym.ID,
							EdgeType: "implements",
							Weight:   10, // Higher weight for implementations
						}
						r.edges = append(r.edges, edge)
					}
				}
			}

			return true
		})
	}

	return nil
}

// extractCallEdges extracts function call relationships
func (r *RelationExtractor) extractCallEdges() error {
	if r.pkg.Types == nil || r.pkg.TypesInfo == nil {
		return nil
	}

	info := r.pkg.TypesInfo

	// Build a map of AST nodes to their enclosing function
	enclosingFunc := make(map[ast.Node]types.Object)

	for _, astFile := range r.pkg.Syntax {
		// First pass: find all function declarations
		ast.Inspect(astFile, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if ok {
				obj := info.ObjectOf(funcDecl.Name)
				if obj != nil {
					enclosingFunc[funcDecl] = obj
				}
			}
			return true
		})

		// Second pass: find call expressions
		ast.Inspect(astFile, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Find the calling function
			var caller types.Object
			for node, obj := range enclosingFunc {
				if r.isNodeInRange(callExpr, node, astFile) {
					caller = obj
					break
				}
			}

			if caller == nil {
				return true
			}

			// Get the called function
			called := r.getCalledFunction(callExpr, info)
			if called == nil {
				return true
			}

			// Create edge
			fromSym := r.findSymbolByObj(caller)
			toSym := r.findSymbolByObj(called)

			if fromSym != nil && toSym != nil && fromSym.ID != toSym.ID {
				edge := &Edge{
					FromID:   fromSym.ID,
					ToID:     toSym.ID,
					EdgeType: "calls",
					Weight:   5, // Medium weight for calls
				}
				r.edges = append(r.edges, edge)
			}

			return true
		})
	}

	return nil
}

// extractFieldEdges extracts struct field and embedding relationships
func (r *RelationExtractor) extractFieldEdges() error {
	if r.pkg.Types == nil || r.pkg.TypesInfo == nil {
		return nil
	}

	info := r.pkg.TypesInfo

	// Find struct types and their fields
	for _, astFile := range r.pkg.Syntax {
		ast.Inspect(astFile, func(n ast.Node) bool {
			typeSpec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return true
			}

			// Get the parent struct symbol
			structObj := info.ObjectOf(typeSpec.Name)
			structSym := r.findSymbolByObj(structObj)
			if structSym == nil {
				return true
			}

			// Process each field
			if structType.Fields != nil {
				for _, field := range structType.Fields.List {
					// Check for embedded fields (anonymous)
					if len(field.Names) == 0 {
						// This is an embedded field
						embeddedType := info.TypeOf(field.Type)
						if embeddedType != nil {
							embeddedSym := r.findSymbolByType(embeddedType)
							if embeddedSym != nil {
								edge := &Edge{
									FromID:   structSym.ID,
									ToID:     embeddedSym.ID,
									EdgeType: "embeds",
									Weight:   7, // High weight for embeddings
								}
								r.edges = append(r.edges, edge)
							}
						}
					} else {
						// Regular field - create reference edge
						for range field.Names {
							fieldType := info.TypeOf(field.Type)
							if fieldType != nil {
								fieldTypeSym := r.findSymbolByType(fieldType)
								if fieldTypeSym != nil {
									// Create edge from struct to field type
									edge := &Edge{
										FromID:   structSym.ID,
										ToID:     fieldTypeSym.ID,
										EdgeType: "references",
										Weight:   2, // Low weight for references
									}
									r.edges = append(r.edges, edge)
								}
							}
						}
					}
				}
			}

			return true
		})
	}

	return nil
}

// Helper methods

func (r *RelationExtractor) isStdLib(importPath string) bool {
	// Standard library packages don't contain a dot
	return !strings.Contains(importPath, ".")
}

func (r *RelationExtractor) findSymbolByObj(obj types.Object) *ExtractedSymbol {
	if obj == nil {
		return nil
	}

	// Handle builtin types (nil package)
	if obj.Pkg() == nil {
		return nil
	}

	// Try to find symbol by object name and package
	pkgPath := obj.Pkg().Path()
	objName := obj.Name()
	kind := r.objKindToSymKind(obj)

	// Generate possible IDs
	possibleIDs := []string{
		fmt.Sprintf("%s:%s:%s", pkgPath, kind, objName),
		fmt.Sprintf("pkg:%s", pkgPath),
	}

	for _, id := range possibleIDs {
		if sym, ok := r.symbols[id]; ok {
			return sym
		}
	}

	return nil
}

func (r *RelationExtractor) findSymbolByTypesID(id string) *ExtractedSymbol {
	// Try to parse the ID format from types.Info
	// The ID format is typically "path/to/file:line" or similar
	// We need to match it to our symbol IDs

	// Try to find by name matching
	for _, sym := range r.symbols {
		if strings.Contains(id, sym.Name) {
			return sym
		}
	}

	return nil
}

func (r *RelationExtractor) findSymbolByType(typ types.Type) *ExtractedSymbol {
	if typ == nil {
		return nil
	}

	// Handle named types
	named, ok := typ.(*types.Named)
	if ok {
		obj := named.Obj()
		return r.findSymbolByObj(obj)
	}

	// Handle pointers
	ptr, ok := typ.(*types.Pointer)
	if ok {
		return r.findSymbolByType(ptr.Elem())
	}

	return nil
}

func (r *RelationExtractor) isNodeInRange(node ast.Node, container ast.Node, file *ast.File) bool {
	nodePos := r.pkg.Fset.Position(node.Pos())
	containerPos := r.pkg.Fset.Position(container.Pos())
	containerEnd := r.pkg.Fset.Position(container.End())

	return nodePos.Line >= containerPos.Line && nodePos.Line <= containerEnd.Line
}

func (r *RelationExtractor) getCalledFunction(callExpr *ast.CallExpr, info *types.Info) types.Object {
	// Get the function being called
	funType := info.TypeOf(callExpr.Fun)
	if funType == nil {
		return nil
	}

	// Handle different types of callees
	switch fn := callExpr.Fun.(type) {
	case *ast.Ident:
		return info.ObjectOf(fn)
	case *ast.SelectorExpr:
		// Method call or package function
		sel := info.ObjectOf(fn.Sel)
		if sel != nil {
			return sel
		}
	}

	return nil
}

func (r *RelationExtractor) objKindToSymKind(obj types.Object) string {
	switch obj.(type) {
	case *types.Func:
		if sig, ok := obj.Type().(*types.Signature); ok && sig.Recv() != nil {
			return "method"
		}
		return "func"
	case *types.TypeName:
		return "type"
	case *types.Var:
		return "var"
	default:
		return "symbol"
	}
}

// GetEdges returns all extracted edges
func (r *RelationExtractor) GetEdges() []*Edge {
	return r.edges
}

// GetEdgesByType returns edges of a specific type
func (r *RelationExtractor) GetEdgesByType(edgeType string) []*Edge {
	var filtered []*Edge
	for _, edge := range r.edges {
		if edge.EdgeType == edgeType {
			filtered = append(filtered, edge)
		}
	}
	return filtered
}

// ConvertToStoreEdges converts extractor edges to store edges
func (r *RelationExtractor) ConvertToStoreEdges() []Edge {
	storeEdges := make([]Edge, len(r.edges))
	for i, edge := range r.edges {
		storeEdges[i] = Edge{
			FromID:     edge.FromID,
			ToID:       edge.ToID,
			EdgeType:   edge.EdgeType,
			Weight:     edge.Weight,
			ImportPath: edge.ImportPath,
		}
	}
	return storeEdges
}
