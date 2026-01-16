package embedding

import (
	"testing"
)

func TestSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 1, 1},
			b:        []float32{-1, -1, -1},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1.1, 2.1, 3.1},
			expected: 0.999, // Approximately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Similarity(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("Similarity() = %v, want %v (diff: %v)", result, tt.expected, diff)
			}
		})
	}
}

func TestL2Distance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "different vectors",
			a:        []float32{0, 0, 0},
			b:        []float32{3, 4, 0},
			expected: 5.0,
		},
		{
			name:     "unit distance",
			a:        []float32{0, 0},
			b:        []float32{1, 0},
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := L2Distance(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("L2Distance() = %v, want %v (diff: %v)", result, tt.expected, diff)
			}
		})
	}
}

func TestSimilarityPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for dimension mismatch")
		}
	}()

	Similarity([]float32{1, 2}, []float32{1, 2, 3})
}

func TestL2DistancePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for dimension mismatch")
		}
	}()

	L2Distance([]float32{1, 2}, []float32{1, 2, 3})
}
