package semantic

import (
	"strings"
	"testing"

	"github.com/DreamCats/bcindex/internal/ast"
)

func TestGenerator_GeneratePackageCard(t *testing.T) {
	generator := NewGenerator()

	// Create a mock package symbol
	pkgSym := &ast.ExtractedSymbol{
		ID:          "pkg:github.com/test/service",
		Name:        "service",
		Kind:        "package",
		PackagePath: "github.com/test/service",
		PackageName: "service",
		Exported:    true,
	}

	// Create mock symbols
	symbols := []*ast.ExtractedSymbol{
		{
			ID:          "github.com/test/service:func:CreateOrder",
			Name:        "CreateOrder",
			Kind:        "func",
			PackagePath: "github.com/test/service",
			PackageName: "service",
			Signature:   "func CreateOrder(req *CreateOrderRequest) (*Order, error)",
			Exported:    true,
		},
		{
			ID:          "github.com/test/service:struct:Order",
			Name:        "Order",
			Kind:        "struct",
			PackagePath: "github.com/test/service",
			PackageName: "service",
			Signature:   "type Order struct",
			Exported:    true,
		},
		{
			ID:          "github.com/test/service:struct:OrderService",
			Name:        "OrderService",
			Kind:        "struct",
			PackagePath: "github.com/test/service",
			PackageName: "service",
			Signature:   "type OrderService struct",
			Exported:    true,
		},
	}

	imports := []string{
		"github.com/test/domain",
		"github.com/test/repo",
		"context",
	}

	card := generator.GeneratePackageCard(pkgSym, symbols, imports)

	t.Logf("Generated Package Card:\n%s", card)

	// Verify card contains expected sections
	expectedSections := []string{
		"Role:",
		"Responsibilities:",
		"Key Types:",
		"Entry Points:",
	}

	for _, section := range expectedSections {
		if !contains(card, section) {
			t.Errorf("Expected card to contain section '%s'", section)
		}
	}

	// Verify role inference
	if !contains(card, "service") {
		t.Error("Expected card to infer 'service' role")
	}

	// Verify key types are present
	if !contains(card, "Order") {
		t.Error("Expected card to contain 'Order' type")
	}

	// Verify entry points
	if !contains(card, "CreateOrder") {
		t.Error("Expected card to contain 'CreateOrder' entry point")
	}
}

func TestGenerator_GenerateSymbolCard(t *testing.T) {
	generator := NewGenerator()

	pkgCard := `Role: domain service
Responsibilities: business logic coordination
Key Types: Order, OrderService
Entry Points: CreateOrder, UpdateOrder`

	sym := &ast.ExtractedSymbol{
		ID:          "github.com/test/service:func:CreateOrder",
		Name:        "CreateOrder",
		Kind:        "func",
		PackagePath: "github.com/test/service",
		PackageName: "service",
		Signature:   "func CreateOrder(req *CreateOrderRequest) (*Order, error)",
		DocComment:  "CreateOrder creates a new order with validation",
		Exported:    true,
	}

	card := generator.GenerateSymbolCard(sym, pkgCard)

	t.Logf("Generated Symbol Card:\n%s", card)

	// Verify card contains expected sections
	expectedSections := []string{
		"Signature:",
		"Kind:",
		"Documentation:",
		"Package Context:",
	}

	for _, section := range expectedSections {
		if !contains(card, section) {
			t.Errorf("Expected card to contain section '%s'", section)
		}
	}

	// Verify signature is present
	if !contains(card, "func CreateOrder") {
		t.Error("Expected card to contain function signature")
	}

	// Verify documentation is present
	if !contains(card, "CreateOrder creates") {
		t.Error("Expected card to contain documentation")
	}
}

func TestGenerator_RoleInference(t *testing.T) {
	generator := NewGenerator()

	tests := []struct {
		name      string
		pkgPath   string
		expectRole string
	}{
		{"repository layer", "github.com/test/repo/order", "data access"},
		{"domain layer", "github.com/test/domain/model", "domain model"},
		{"service layer", "github.com/test/service/order", "application service"},
		{"api layer", "github.com/test/api/handler", "api transport"},
		{"infrastructure", "github.com/test/infra/config", "infrastructure"},
		{"utility", "github.com/test/util/helper", "utility"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgSym := &ast.ExtractedSymbol{
				ID:          "pkg:" + tt.pkgPath,
				Name:        extractDirName(tt.pkgPath),
				Kind:        "package",
				PackagePath: tt.pkgPath,
				PackageName: extractDirName(tt.pkgPath),
				Exported:    true,
			}

			symbols := []*ast.ExtractedSymbol{}
			imports := []string{}

			card := generator.GeneratePackageCard(pkgSym, symbols, imports)

			if !contains(card, tt.expectRole) {
				t.Errorf("Expected role '%s', got card:\n%s", tt.expectRole, card)
			}
		})
	}
}

func TestGenerator_ResponsibilitiesGeneration(t *testing.T) {
	generator := NewGenerator()

	tests := []struct {
		name          string
		pkgPath       string
		expectResp    []string
	}{
		{
			"order service",
			"github.com/test/service/order",
			[]string{"order", "business logic"},
		},
		{
			"user auth",
			"github.com/test/service/auth",
			[]string{"user", "authentication"},
		},
		{
			"repository",
			"github.com/test/repo/order",
			[]string{"data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgSym := &ast.ExtractedSymbol{
				ID:          "pkg:" + tt.pkgPath,
				Name:        extractDirName(tt.pkgPath),
				Kind:        "package",
				PackagePath: tt.pkgPath,
				PackageName: extractDirName(tt.pkgPath),
				Exported:    true,
			}

			symbols := []*ast.ExtractedSymbol{}
			imports := []string{}

			card := generator.GeneratePackageCard(pkgSym, symbols, imports)

			// Check for expected responsibility keywords
			for _, keyword := range tt.expectResp {
				if !contains(card, keyword) {
					t.Errorf("Expected responsibility to contain '%s', got card:\n%s", keyword, card)
				}
			}
		})
	}
}

func TestGenerator_EntryPointDetection(t *testing.T) {
	generator := NewGenerator()

	tests := []struct {
		name     string
		funcName string
		expect   bool
	}{
		{"Create function", "CreateOrder", true},
		{"Get function", "GetOrder", true},
		{"Handle function", "HandleRequest", true},
		{"Process function", "ProcessData", true},
		{"Helper function", "calculateTotal", false},
		{"Internal function", "validateInput", false},
		{"Init function", "InitService", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sym := &ast.ExtractedSymbol{
				Name:     tt.funcName,
				Kind:     "func",
				Exported: true,
			}

			result := generator.isEntryPoint(sym)
			if result != tt.expect {
				t.Errorf("isEntryPoint(%s) = %v, expected %v", tt.funcName, result, tt.expect)
			}
		})
	}
}

func TestGenerator_GenerateCardForRealWorldPackages(t *testing.T) {
	generator := NewGenerator()

	// Test a realistic domain package
	domainSym := &ast.ExtractedSymbol{
		ID:          "pkg:github.com/ecommerce/order/domain",
		Name:        "domain",
		Kind:        "package",
		PackagePath: "github.com/ecommerce/order/domain",
		PackageName: "domain",
		Exported:    true,
	}

	domainSymbols := []*ast.ExtractedSymbol{
		{
			Name:      "Order",
			Kind:      "struct",
			Exported:  true,
			Signature: "type Order struct",
		},
		{
			Name:      "OrderRepository",
			Kind:      "interface",
			Exported:  true,
			Signature: "type OrderRepository interface",
		},
		{
			Name:      "Create",
			Kind:      "func",
			Exported:  true,
			Signature: "func Create(req *CreateOrderRequest) (*Order, error)",
		},
	}

	domainCard := generator.GeneratePackageCard(domainSym, domainSymbols, []string{})
	t.Logf("Domain Package Card:\n%s", domainCard)

	// Verify domain role is detected
	if !contains(strings.ToLower(domainCard), "domain") {
		t.Error("Expected domain role to be detected")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
