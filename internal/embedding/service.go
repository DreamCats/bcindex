package embedding

import (
	"context"
	"fmt"
	"math"

	"github.com/DreamCats/bcindex/internal/config"
)

// Service provides embedding generation functionality
type Service struct {
	cfg     *config.EmbeddingConfig
	client  Client
}

// Client is the interface for embedding API clients
type Client interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// NewService creates a new embedding service
func NewService(cfg *config.EmbeddingConfig) (*Service, error) {
	svc := &Service{cfg: cfg}

	var client Client
	var err error

	switch cfg.Provider {
	case "volcengine":
		client, err = NewVolcEngineClient(cfg)
	case "openai":
		client, err = NewOpenAIClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create embedding client: %w", err)
	}

	svc.client = client
	return svc, nil
}

// Embed generates an embedding for a single text
func (s *Service) Embed(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("cannot embed empty text")
	}
	return s.client.Embed(ctx, text)
}

// EmbedBatch generates embeddings for multiple texts
func (s *Service) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Filter out empty texts
	validTexts := make([]string, 0, len(texts))
	validIndices := make([]int, 0, len(texts))
	for i, text := range texts {
		if text != "" {
			validTexts = append(validTexts, text)
			validIndices = append(validIndices, i)
		}
	}

	if len(validTexts) == 0 {
		return nil, fmt.Errorf("no valid texts to embed")
	}

	// Process in batches
	batchSize := s.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	results := make([][]float32, len(texts))

	for i := 0; i < len(validTexts); i += batchSize {
		end := i + batchSize
		if end > len(validTexts) {
			end = len(validTexts)
		}

		batch := validTexts[i:end]
		embeddings, err := s.client.EmbedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to embed batch %d-%d: %w", i, end, err)
		}

		// Map results back to original indices
		for j, emb := range embeddings {
			results[validIndices[i+j]] = emb
		}
	}

	return results, nil
}

// Dimensions returns the dimension of the embeddings
func (s *Service) Dimensions() int {
	return s.client.Dimensions()
}

// Similarity computes cosine similarity between two vectors
func Similarity(a, b []float32) float32 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("vector dimension mismatch: %d vs %d", len(a), len(b)))
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// L2Distance computes L2 (Euclidean) distance between two vectors
func L2Distance(a, b []float32) float32 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("vector dimension mismatch: %d vs %d", len(a), len(b)))
	}

	var sum float32
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return float32(math.Sqrt(float64(sum)))
}
