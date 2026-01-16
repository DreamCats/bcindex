package retrieval

import (
	"testing"

	"github.com/DreamCats/bcindex/internal/store"
)

// TestEvidenceBuilder_BuildPackageCards tests package aggregation
func TestEvidenceBuilder_BuildPackageCards(t *testing.T) {
	// Create a builder with stores (we'll use nil for now as we're testing logic)
	builder := &EvidenceBuilder{
		maxPackages: 3,
		maxSymbols:  10,
	}

	// Create test results
	results := []SearchResult{
		{
			Symbol: &store.Symbol{
				ID:          "sym1",
				Name:        "HandleOrder",
				Kind:        "func",
				PackagePath: "github.com/test/api/handler/order",
				Exported:    true,
			},
			CombinedScore: 0.9,
			GraphFeatures: &GraphFeatures{
				Layer:   "handler",
				IsEntry: true,
			},
		},
		{
			Symbol: &store.Symbol{
				ID:          "sym2",
				Name:        "OrderService",
				Kind:        "struct",
				PackagePath: "github.com/test/service/order",
				Exported:    true,
			},
			CombinedScore: 0.8,
			GraphFeatures: &GraphFeatures{
				Layer: "service",
			},
		},
		{
			Symbol: &store.Symbol{
				ID:          "sym3",
				Name:        "CreateOrder",
				Kind:        "method",
				PackagePath: "github.com/test/service/order",
				Exported:    true,
			},
			CombinedScore: 0.7,
		},
	}

	cards := builder.buildPackageCards(results)

	if len(cards) == 0 {
		t.Fatal("expected at least one package card")
	}

	// Should have 2 packages (handler and service)
	if len(cards) != 2 {
		t.Errorf("expected 2 package cards, got %d", len(cards))
	}

	// First card should be service (more results = higher total score: 0.8+0.7=1.5 vs 0.9+0.1=1.0)
	if cards[0].Path != "github.com/test/service/order" {
		t.Errorf("expected first card to be service package, got %s", cards[0].Path)
	}
}

// TestEvidenceBuilder_DetectPackageRole tests role detection
func TestEvidenceBuilder_DetectPackageRole(t *testing.T) {
	builder := &EvidenceBuilder{}

	tests := []struct {
		name     string
		pkgPath  string
		expected string
	}{
		{
			name:     "handler package",
			pkgPath:  "github.com/test/api/handler/order",
			expected: "interface/http",
		},
		{
			name:     "service package",
			pkgPath:  "github.com/test/service/order",
			expected: "application/business",
		},
		{
			name:     "repository package",
			pkgPath:  "github.com/test/repository/order",
			expected: "infrastructure/persistence",
		},
		{
			name:     "domain package",
			pkgPath:  "github.com/test/domain/order",
			expected: "domain/model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.detectPackageRole(tt.pkgPath, []SearchResult{})
			if result != tt.expected {
				t.Errorf("detectPackageRole() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestEvidenceBuilder_BuildSymbolCards tests symbol card building
func TestEvidenceBuilder_BuildSymbolCards(t *testing.T) {
	builder := &EvidenceBuilder{
		maxSymbols: 5,
	}

	results := []SearchResult{
		{
			Symbol: &store.Symbol{
				ID:          "sym1",
				Name:        "HandleOrder",
				Kind:        "func",
				Signature:   "func HandleOrder(w http.ResponseWriter, r *http.Request)",
				PackagePath: "github.com/test/api/handler",
				FilePath:    "/test/handler.go",
				LineStart:   10,
				SemanticText: "This is a test semantic text for the order handler function that processes incoming requests",
			},
			CombinedScore: 0.9,
			Reason:        []string{"High relevance", "Entry point"},
		},
		{
			Symbol: &store.Symbol{
				ID:          "sym2",
				Name:        "OrderService",
				Kind:        "struct",
				PackagePath: "github.com/test/service",
				FilePath:    "/test/service.go",
				LineStart:   20,
			},
			CombinedScore: 0.8,
			Reason:        []string{"Core service"},
		},
	}

	cards := builder.buildSymbolCards(results)

	if len(cards) != 2 {
		t.Errorf("expected 2 symbol cards, got %d", len(cards))
	}

	// Check first card
	if cards[0].ID != "sym1" {
		t.Errorf("expected first card ID to be sym1, got %s", cards[0].ID)
	}
	if cards[0].Name != "HandleOrder" {
		t.Errorf("expected first card name to be HandleOrder, got %s", cards[0].Name)
	}
	if cards[0].Kind != "func" {
		t.Errorf("expected first card kind to be func, got %s", cards[0].Kind)
	}

	// First card should have snippet (top 3 get snippets)
	if cards[0].Snippet == "" {
		t.Error("expected first card to have snippet")
	}

	// Second card should NOT have snippet (only top 3)
	if cards[1].Snippet != "" {
		t.Error("expected second card to not have snippet")
	}
}

// TestEvidenceBuilder_GeneratePackageSummary tests summary generation
func TestEvidenceBuilder_GeneratePackageSummary(t *testing.T) {
	builder := &EvidenceBuilder{}

	results := []SearchResult{
		{
			Symbol: &store.Symbol{
				Name:        "HandleOrder",
				Kind:        "func",
				SemanticText: "Order management service with CRUD operations",
			},
		},
		{
			Symbol: &store.Symbol{
				Name: "OrderService",
				Kind: "struct",
			},
		},
		{
			Symbol: &store.Symbol{
				Name: "CreateOrder",
				Kind: "method",
			},
		},
	}

	summary := builder.generatePackageSummary("github.com/test/service", results)

	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Should contain symbol count
	if !containsString(summary, "3 symbols") {
		t.Errorf("expected summary to contain '3 symbols', got: %s", summary)
	}

	// Should contain semantic text
	if !containsString(summary, "Order management") {
		t.Errorf("expected summary to contain semantic text, got: %s", summary)
	}
}

// TestEvidenceBuilder_BuildPackageCard tests individual package card building
func TestEvidenceBuilder_BuildPackageCard(t *testing.T) {
	builder := &EvidenceBuilder{}

	results := []SearchResult{
		{
			Symbol: &store.Symbol{
				ID:          "sym1",
				Name:        "HandleOrder",
				Kind:        "func",
				PackagePath: "github.com/test/api/handler/order",
				Exported:    true,
			},
			CombinedScore: 0.9,
			GraphFeatures: &GraphFeatures{
				Layer:   "handler",
				IsEntry: true,
			},
		},
	}

	card := builder.buildPackageCard("github.com/test/api/handler/order", results)

	if card.Path != "github.com/test/api/handler/order" {
		t.Errorf("expected path to be github.com/test/api/handler/order, got %s", card.Path)
	}

	if card.Role != "interface/http" {
		t.Errorf("expected role to be interface/http, got %s", card.Role)
	}

	if len(card.KeySymbols) == 0 {
		t.Error("expected at least one key symbol")
	}

	if len(card.Why) == 0 {
		t.Error("expected at least one reason")
	}
}

// TestEvidenceBuilder_TruncateToLines tests line truncation
func TestEvidenceBuilder_TruncateToLines(t *testing.T) {
	builder := &EvidenceBuilder{}

	content := "line1\nline2\nline3\nline4\nline5"

	// Truncate to 3 lines
	result := builder.truncateToLines(content, 3)

	expected := "line1\nline2\nline3"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}

	// No truncation needed
	result = builder.truncateToLines(content, 10)
	if result != content {
		t.Errorf("expected no truncation, got '%s'", result)
	}
}

// TestEvidenceBuilder_CountSnippetLines tests line counting
func TestEvidenceBuilder_CountSnippetLines(t *testing.T) {
	builder := &EvidenceBuilder{}

	snippets := []store.CodeSnippet{
		{StartLine: 1, EndLine: 10},
		{StartLine: 20, EndLine: 39},
		{StartLine: 50, EndLine: 79},
	}

	total := builder.countSnippetLines(snippets)

	// (10-1+1) + (39-20+1) + (79-50+1) = 10 + 20 + 30 = 60
	if total != 60 {
		t.Errorf("expected 60 total lines, got %d", total)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestEvidenceBuilder_BuildFullEvidencePack tests the full Build method
func TestEvidenceBuilder_BuildFullEvidencePack(t *testing.T) {
	// This is an integration-style test that verifies the structure
	// We can't fully test without real stores, but we can verify the flow

	builder := &EvidenceBuilder{
		maxPackages: 3,
		maxSymbols:  5,
		maxSnippets: 3,
		maxLines:    100,
	}

	results := []SearchResult{
		{
			Symbol: &store.Symbol{
				ID:          "sym1",
				Name:        "HandleOrder",
				Kind:        "func",
				Signature:   "func HandleOrder(w http.ResponseWriter, r *http.Request)",
				PackagePath: "github.com/test/api/handler/order",
				FilePath:    "/test/handler.go",
				LineStart:   10,
				LineEnd:     20,
				Exported:    true,
				SemanticText: "Handles order creation requests",
			},
			CombinedScore: 0.9,
			Reason:        []string{"High relevance"},
			GraphFeatures: &GraphFeatures{
				Layer:   "handler",
				IsEntry: true,
			},
		},
	}

	pack, err := builder.Build("test query", results)

	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	if pack.Query != "test query" {
		t.Errorf("expected query to be 'test query', got '%s'", pack.Query)
	}

	// Should have at least one package
	if len(pack.TopPackages) == 0 {
		t.Error("expected at least one package card")
	}

	// Should have at least one symbol
	if len(pack.TopSymbols) == 0 {
		t.Error("expected at least one symbol card")
	}

	// Metadata should be set
	if pack.Metadata.TotalSymbols != 1 {
		t.Errorf("expected TotalSymbols to be 1, got %d", pack.Metadata.TotalSymbols)
	}
}
