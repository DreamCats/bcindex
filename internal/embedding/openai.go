package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DreamCats/bcindex/internal/config"
)

// OpenAIClient implements Client for OpenAI's embedding API
type OpenAIClient struct {
	apiKey string
	model  string
	client *http.Client
}

// OpenAIEmbeddingRequest is the request format for OpenAI API
type OpenAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// OpenAIEmbeddingResponse is the response from OpenAI API
type OpenAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
		Object    string    `json:"object"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// NewOpenAIClient creates a new OpenAI embedding client
func NewOpenAIClient(cfg *config.EmbeddingConfig) (*OpenAIClient, error) {
	apiKey := cfg.OpenAIAPIKey
	if apiKey == "" {
		return nil, fmt.Errorf("openai api_key is required")
	}

	model := cfg.OpenAIModel
	if model == "" {
		model = "text-embedding-3-small"
	}

	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Embed generates an embedding for a single text
func (c *OpenAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts
func (c *OpenAIClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	req := OpenAIEmbeddingRequest{
		Input: texts,
		Model: c.model,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/embeddings", bytesReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp OpenAIEmbeddingResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(apiResp.Data))
	}

	embeddings := make([][]float32, len(texts))
	for _, data := range apiResp.Data {
		if data.Index < 0 || data.Index >= len(texts) {
			return nil, fmt.Errorf("invalid embedding index: %d", data.Index)
		}
		embeddings[data.Index] = data.Embedding
	}

	return embeddings, nil
}

// Dimensions returns the dimension of the embeddings
func (c *OpenAIClient) Dimensions() int {
	// text-embedding-3-small defaults to 1536
	// text-embedding-3-large defaults to 3072
	// This is a simplified implementation
	return 1536
}
