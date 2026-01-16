package semantic

import (
	"fmt"
	"strings"

	"github.com/DreamCats/bcindex/internal/ast"
)

// Generator generates semantic descriptions for packages and symbols
type Generator struct {
	// Role inference rules
	roleRules []RoleRule
}

// RoleRule defines a rule for inferring package role
type RoleRule struct {
	Name      string
	Patterns  []string // Directory name patterns
	Import    []string // Import patterns
	Infer     func(pkg *ast.ExtractedSymbol, info *PackageInfo) string
}

// PackageInfo holds aggregated information about a package
type PackageInfo struct {
	Path           string
	Name           string
	DirName        string
	PathParts      []string // All parts of the path for role inference
	Symbols        []*ast.ExtractedSymbol
	Exports        []*ast.ExtractedSymbol
	Imports        []string
	ImportedBy     []string
	HasInterfaces  bool
	HasStructs     bool
	HasMethods     bool
	KeyTypes       []string
	KeyFuncs       []string
}

// NewGenerator creates a new semantic generator
func NewGenerator() *Generator {
	g := &Generator{
		roleRules: buildDefaultRoleRules(),
	}
	return g
}

// GeneratePackageCard generates a semantic card for a package
func (g *Generator) GeneratePackageCard(pkgSym *ast.ExtractedSymbol, symbols []*ast.ExtractedSymbol, imports []string) string {
	info := g.buildPackageInfo(pkgSym, symbols, imports)

	// Infer role
	role := g.inferRole(info)

	// Generate responsibilities
	responsibilities := g.generateResponsibilities(info)

	// Extract key types and functions
	keyTypes := g.extractKeyTypes(info)
	keyFuncs := g.extractKeyFuncs(info)

	// Format output
	var card strings.Builder
	card.WriteString(fmt.Sprintf("Role: %s\n", role))
	card.WriteString(fmt.Sprintf("Responsibilities: %s\n", responsibilities))
	card.WriteString(fmt.Sprintf("Key Types: %s\n", strings.Join(keyTypes, ", ")))
	card.WriteString(fmt.Sprintf("Entry Points: %s\n", strings.Join(keyFuncs, ", ")))

	if len(info.Imports) > 0 {
		// Show unique external imports
		externImports := g.filterExternalImports(info.Imports)
		if len(externImports) > 0 {
			card.WriteString(fmt.Sprintf("Dependencies: %s\n", strings.Join(externImports, ", ")))
		}
	}

	return card.String()
}

// GenerateSymbolCard generates a semantic card for a symbol
func (g *Generator) GenerateSymbolCard(sym *ast.ExtractedSymbol, pkgCard string) string {
	var card strings.Builder

	// Add signature
	card.WriteString(fmt.Sprintf("Signature: %s\n", sym.Signature))

	// Add kind
	card.WriteString(fmt.Sprintf("Kind: %s\n", sym.Kind))

	// Add documentation
	if sym.DocComment != "" {
		card.WriteString(fmt.Sprintf("Documentation: %s\n", sym.DocComment))
	}

	// Add package context
	if pkgCard != "" {
		card.WriteString(fmt.Sprintf("Package Context:\n%s\n", pkgCard))
	}

	// Add additional context based on symbol type
	switch sym.Kind {
	case "method", "func":
		card.WriteString(g.generateFuncContext(sym))
	case "struct", "interface":
		card.WriteString(g.generateTypeContext(sym))
	}

	return card.String()
}

// buildPackageInfo aggregates information about a package
func (g *Generator) buildPackageInfo(pkgSym *ast.ExtractedSymbol, symbols []*ast.ExtractedSymbol, imports []string) *PackageInfo {
	info := &PackageInfo{
		Path:      pkgSym.PackagePath,
		Name:      pkgSym.PackageName,
		DirName:   extractDirName(pkgSym.PackagePath),
		PathParts: strings.Split(pkgSym.PackagePath, "/"),
		Symbols:   symbols,
		Imports:   imports,
	}

	// Find exported symbols
	for _, sym := range symbols {
		if sym.Exported && sym.Kind != "package" && sym.Kind != "file" {
			info.Exports = append(info.Exports, sym)

			switch sym.Kind {
			case "interface":
				info.HasInterfaces = true
				info.KeyTypes = append(info.KeyTypes, sym.Name)
			case "struct":
				info.HasStructs = true
				info.KeyTypes = append(info.KeyTypes, sym.Name)
			case "method", "func":
				info.HasMethods = true
				if g.isEntryPoint(sym) {
					info.KeyFuncs = append(info.KeyFuncs, sym.Name)
				}
			}
		}
	}

	return info
}

// inferRole infers the role of a package based on naming patterns and imports
func (g *Generator) inferRole(info *PackageInfo) string {
	// Check each role rule
	for _, rule := range g.roleRules {
		if g.matchRoleRule(rule, info) {
			if rule.Infer != nil {
				return rule.Infer(nil, info)
			}
			return rule.Name
		}
	}

	// Default inference
	return g.defaultRoleInference(info)
}

// matchRoleRule checks if a role rule matches the package info
func (g *Generator) matchRoleRule(rule RoleRule, info *PackageInfo) bool {
	// Check directory patterns in all path parts
	for _, part := range info.PathParts {
		partLower := strings.ToLower(part)
		for _, pattern := range rule.Patterns {
			if strings.Contains(partLower, pattern) {
				return true
			}
		}
	}

	// Check import patterns
	for _, imp := range info.Imports {
		for _, pattern := range rule.Import {
			if strings.Contains(imp, pattern) {
				return true
			}
		}
	}

	return false
}

// defaultRoleInference provides default role inference
func (g *Generator) defaultRoleInference(info *PackageInfo) string {
	// Check all path parts for role indicators
	for _, part := range info.PathParts {
		partLower := strings.ToLower(part)

		// Check for domain layer (more specific check first)
		if strings.Contains(partLower, "domain") {
			if info.HasInterfaces {
				return "domain interface"
			}
			return "domain model"
		}

		// Check for model specifically
		if strings.Contains(partLower, "model") {
			return "domain model"
		}

		// Check for repository/data layer
		if strings.Contains(partLower, "repo") || strings.Contains(partLower, "repository") {
			return "data access"
		}

		if strings.Contains(partLower, "data") || strings.Contains(partLower, "db") {
			return "data access"
		}

		// Check for service layer
		if strings.Contains(partLower, "service") || strings.Contains(partLower, "svc") {
			return "application service"
		}

		// Check for API/transport layer
		if strings.Contains(partLower, "api") || strings.Contains(partLower, "http") || strings.Contains(partLower, "grpc") {
			return "api transport"
		}

		if strings.Contains(partLower, "handler") || strings.Contains(partLower, "controller") {
			return "api transport"
		}

		// Check for infrastructure
		if strings.Contains(partLower, "infra") || strings.Contains(partLower, "config") {
			return "infrastructure"
		}

		// Check for utilities
		if strings.Contains(partLower, "util") || strings.Contains(partLower, "helper") || strings.Contains(partLower, "common") {
			return "utility"
		}
	}

	// Default based on content
	if info.HasInterfaces && info.HasStructs {
		return "domain logic"
	}

	if info.HasMethods && !info.HasInterfaces {
		return "business logic"
	}

	return "general"
}

// generateResponsibilities generates responsibility description
func (g *Generator) generateResponsibilities(info *PackageInfo) string {
	var resp []string

	// Check all path parts for responsibility indicators
	for _, part := range info.PathParts {
		partLower := strings.ToLower(part)

		// Infer responsibilities from directory name
		if strings.Contains(partLower, "user") || strings.Contains(partLower, "auth") {
			resp = append(resp, "user authentication and authorization")
		}

		if strings.Contains(partLower, "order") {
			resp = append(resp, "order lifecycle management")
		}

		if strings.Contains(partLower, "product") {
			resp = append(resp, "product catalog management")
		}

		if strings.Contains(partLower, "payment") {
			resp = append(resp, "payment processing")
		}

		if strings.Contains(partLower, "repo") || strings.Contains(partLower, "repository") {
			resp = append(resp, "data persistence and retrieval")
		}

		if strings.Contains(partLower, "data") || strings.Contains(partLower, "db") {
			resp = append(resp, "data persistence and retrieval")
		}

		if strings.Contains(partLower, "api") || strings.Contains(partLower, "http") {
			resp = append(resp, "HTTP request handling")
		}

		if strings.Contains(partLower, "service") || strings.Contains(partLower, "svc") {
			resp = append(resp, "business logic coordination")
		}

		if strings.Contains(partLower, "util") || strings.Contains(partLower, "helper") {
			resp = append(resp, "utility functions")
		}
	}

	// If no specific responsibilities found, use generic ones
	if len(resp) == 0 {
		if info.HasInterfaces {
			resp = append(resp, "interface definitions")
		}
		if info.HasStructs {
			resp = append(resp, "data structures")
		}
		if info.HasMethods {
			resp = append(resp, "business operations")
		}
		if len(resp) == 0 {
			resp = append(resp, "general functionality")
		}
	}

	return strings.Join(resp, ", ")
}

// extractKeyTypes extracts key type names
func (g *Generator) extractKeyTypes(info *PackageInfo) []string {
	if len(info.KeyTypes) > 0 {
		return info.KeyTypes
	}
	return []string{"N/A"}
}

// extractKeyFuncs extracts key function names
func (g *Generator) extractKeyFuncs(info *PackageInfo) []string {
	if len(info.KeyFuncs) > 0 {
		return info.KeyFuncs
	}

	// If no explicit entry points, use exported functions
	var funcs []string
	for _, sym := range info.Exports {
		if sym.Kind == "func" || sym.Kind == "method" {
			funcs = append(funcs, sym.Name)
		}
	}

	if len(funcs) > 5 {
		funcs = funcs[:5]
	}

	if len(funcs) > 0 {
		return funcs
	}

	return []string{"N/A"}
}

// isEntryPoint determines if a function is an entry point
func (g *Generator) isEntryPoint(sym *ast.ExtractedSymbol) bool {
	name := sym.Name

	// Common entry point patterns
	entryPointPrefixes := []string{"Create", "New", "Get", "List", "Update", "Delete", "Handle", "Process", "Run", "Start", "Stop", "Init"}

	for _, prefix := range entryPointPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// filterExternalImports filters external (non-stdlib) imports
func (g *Generator) filterExternalImports(imports []string) []string {
	var extern []string
	for _, imp := range imports {
		if strings.Contains(imp, ".") && !strings.Contains(imp, "/vendor/") {
			extern = append(extern, imp)
		}
	}
	return extern
}

// generateFuncContext generates additional context for functions/methods
func (g *Generator) generateFuncContext(sym *ast.ExtractedSymbol) string {
	var ctx strings.Builder

	// Extract parameter and return info from signature
	if strings.Contains(sym.Signature, "error") {
		ctx.WriteString("Returns error for failure cases\n")
	}

	return ctx.String()
}

// generateTypeContext generates additional context for types
func (g *Generator) generateTypeContext(sym *ast.ExtractedSymbol) string {
	var ctx strings.Builder

	if sym.Kind == "interface" {
		ctx.WriteString("Interface defining behavior contract\n")
	} else if sym.Kind == "struct" {
		ctx.WriteString("Data structure\n")
	}

	return ctx.String()
}

// extractDirName extracts the directory name from package path
func extractDirName(pkgPath string) string {
	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkgPath
}

// buildDefaultRoleRules builds the default role inference rules
func buildDefaultRoleRules() []RoleRule {
	return []RoleRule{
		{
			Name:     "data access",
			Patterns: []string{"repo", "repository"},
			Import:   []string{"database", "sql", "gorm"},
			Infer: func(pkg *ast.ExtractedSymbol, info *PackageInfo) string {
				return "data access"
			},
		},
		{
			Name:     "data access",
			Patterns: []string{"data", "db"},
			Import:   []string{"database", "sql", "gorm"},
		},
		{
			Name:     "domain model",
			Patterns: []string{"domain", "model"},
			Import:   []string{},
		},
		{
			Name:     "application service",
			Patterns: []string{"service", "svc"},
			Import:   []string{},
		},
		{
			Name:     "api transport",
			Patterns: []string{"api", "http", "handler", "controller", "grpc", "rest"},
			Import:   []string{"http", "gin", "echo", "grpc"},
		},
		{
			Name:     "infrastructure",
			Patterns: []string{"infra", "config", "setting"},
			Import:   []string{"config", "viper"},
		},
		{
			Name:     "utility",
			Patterns: []string{"util", "helper", "common", "base"},
			Import:   []string{},
		},
		{
			Name:     "client",
			Patterns: []string{"client", "rpc"},
			Import:   []string{"rpc", "grpc"},
		},
	}
}
