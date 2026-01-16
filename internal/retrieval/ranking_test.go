package retrieval

import (
	"testing"

	"github.com/DreamCats/bcindex/internal/store"
)

// Test GraphRanker helper methods

func TestGraphRanker_DetectLayer(t *testing.T) {
	ranker := &GraphRanker{}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"handler layer", "github.com/test/api/handler/order", "handler"},
		{"service layer", "github.com/test/service/order", "service"},
		{"repository layer", "github.com/test/repository/order", "repository"},
		{"domain layer", "github.com/test/domain/order", "domain"},
		{"middleware layer", "github.com/test/middleware/auth", "middleware"},
		{"util layer", "github.com/test/util/string", "util"},
		{"unknown layer", "github.com/test/random/pkg", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sym := &store.Symbol{
				ID:          "test",
				PackagePath: tt.path,
			}
			result := ranker.detectLayer(sym)
			if result != tt.expected {
				t.Errorf("detectLayer() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGraphRanker_IsEntryPoint(t *testing.T) {
	ranker := &GraphRanker{}

	tests := []struct {
		name     string
		symbol   *store.Symbol
		expected bool
	}{
		{
			name: "main function",
			symbol: &store.Symbol{
				ID:       "main",
				Name:     "main",
				Kind:     store.KindFunc,
				Exported: true,
			},
			expected: true,
		},
		{
			name: "HTTP handler",
			symbol: &store.Symbol{
				ID:          "handler1",
				Name:        "HandleOrder",
				Kind:        store.KindFunc,
				Exported:    true,
				PackagePath: "github.com/test/api/handler/order",
				Signature:   "func HandleOrder(w http.ResponseWriter, r *http.Request)",
			},
			expected: true,
		},
		{
			name: "private function",
			symbol: &store.Symbol{
				ID:       "privateFunc",
				Name:     "privateFunc",
				Kind:     store.KindFunc,
				Exported: false,
			},
			expected: false,
		},
		{
			name: "regular exported function",
			symbol: &store.Symbol{
				ID:          "calc",
				Name:        "Calculate",
				Kind:        store.KindFunc,
				Exported:    true,
				PackagePath: "github.com/test/math",
				Signature:   "func Calculate(x, y int) int",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ranker.isEntryPoint(tt.symbol)
			if result != tt.expected {
				t.Errorf("isEntryPoint() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGraphRanker_DetectIntent(t *testing.T) {
	ranker := &GraphRanker{}

	tests := []struct {
		query    string
		expected string
	}{
		{"order design pattern", "design"},
		{"订单状态变更方案", "design"},
		{"system architecture", "design"},
		{"bug fix for order", "implementation"},
		{"错误处理实现", "implementation"},
		{"interface for payment", "extension"},
		{"扩展点在哪里", "extension"},
		{"just a regular query", ""},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ranker.detectIntent(tt.query)
			if result != tt.expected {
				t.Errorf("detectIntent(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}

func TestGraphRanker_GetLayerScore(t *testing.T) {
	ranker := &GraphRanker{}

	tests := []struct {
		layer    string
		minScore float64
	}{
		{"handler", 0.7},
		{"service", 0.8},
		{"repository", 0.3},
		{"domain", 0.5},
		{"unknown", 0.4},
	}

	for _, tt := range tests {
		t.Run(tt.layer, func(t *testing.T) {
			score := ranker.getLayerScore(tt.layer)
			if score < tt.minScore {
				t.Errorf("getLayerScore(%q) = %v, want >= %v", tt.layer, score, tt.minScore)
			}
		})
	}
}

func TestGraphRanker_ReorderWithIntent(t *testing.T) {
	ranker := &GraphRanker{}

	// Create symbols from different layers
	handler := &store.Symbol{
		ID:          "handler1",
		Name:        "HandleOrder",
		Kind:        store.KindFunc,
		PackagePath: "github.com/test/api/handler/order",
	}
	svc := &store.Symbol{
		ID:          "svc1",
		Name:        "OrderService",
		Kind:        store.KindStruct,
		PackagePath: "github.com/test/service/order",
	}
	repo := &store.Symbol{
		ID:          "repo1",
		Name:        "OrderRepo",
		Kind:        store.KindStruct,
		PackagePath: "github.com/test/repository/order",
	}
	iface := &store.Symbol{
		ID:          "iface1",
		Name:        "ServiceInterface",
		Kind:        store.KindInterface,
		PackagePath: "github.com/test/service/order",
	}

	baseResults := []*RankedResult{
		{
			Symbol: repo,
			Features: &GraphFeatures{
				Layer:       "repository",
				IsInterface: false,
			},
		},
		{
			Symbol: svc,
			Features: &GraphFeatures{
				Layer:       "service",
				IsInterface: false,
			},
		},
		{
			Symbol: handler,
			Features: &GraphFeatures{
				Layer:       "handler",
				IsInterface: false,
			},
		},
		{
			Symbol: iface,
			Features: &GraphFeatures{
				Layer:       "service",
				IsInterface: true,
			},
		},
	}

	t.Run("design query prefers service and interface", func(t *testing.T) {
		results := ranker.ReorderWithIntent(baseResults, "order design architecture")
		if len(results) == 0 {
			t.Fatal("no results returned")
		}
		// First result should be interface or service
		first := results[0]
		if first.Features.Layer != "service" && !first.Features.IsInterface {
			t.Errorf("expected service or interface first, got %v (%v)",
				first.Symbol.Name, first.Features.Layer)
		}
	})

	t.Run("implementation query prefers repository", func(t *testing.T) {
		results := ranker.ReorderWithIntent(baseResults, "bug fix in order storage")
		if len(results) == 0 {
			t.Fatal("no results returned")
		}
		// Repository should be ranked higher
		foundRepo := false
		for i, r := range results {
			if r.Symbol.ID == "repo1" && i < 2 {
				foundRepo = true
				break
			}
		}
		if !foundRepo {
			t.Error("repository should be ranked higher for implementation query")
		}
	})

	t.Run("extension query prefers interface", func(t *testing.T) {
		results := ranker.ReorderWithIntent(baseResults, "extension point for payment")
		if len(results) == 0 {
			t.Fatal("no results returned")
		}
		// Interface should be first
		if results[0].Symbol.ID != "iface1" {
			t.Errorf("expected interface first, got %v", results[0].Symbol.Name)
		}
	})
}
