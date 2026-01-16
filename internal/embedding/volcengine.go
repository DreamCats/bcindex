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

// VolcEngineClient implements Client for VolcEngine's multimodal embedding API
type VolcEngineClient struct {
	apiKey     string
	endpoint   string
	model      string
	dimensions int
	client     *http.Client
}

// VolcEngineEmbeddingRequest is the request format for VolcEngine API
type VolcEngineEmbeddingRequest struct {
	Input          []VolcEngineInput `json:"input"`
	Model          string            `json:"model"`
	EncodingFormat string            `json:"encoding_format,omitempty"`
	Dimensions     int               `json:"dimensions,omitempty"`
}

// VolcEngineInput represents an input item for embedding
type VolcEngineInput struct {
	Type string `json:"type"` // "text" | "image_url" | "video_url"
	Text string `json:"text,omitempty"`
}

// VolcEngineEmbeddingResponse is the response from VolcEngine API
type VolcEngineEmbeddingResponse struct {
	ID     string          `json:"id"`
	Model  string          `json:"model"`
	Object string          `json:"object"`
	Data   json.RawMessage `json:"data"`
	Usage  struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Created int64 `json:"created"`
}

type VolcEngineEmbeddingData struct {
	Embedding []float32 `json:"embedding"`
	Object    string    `json:"object"`
}

// NewVolcEngineClient creates a new VolcEngine embedding client
func NewVolcEngineClient(cfg *config.EmbeddingConfig) (*VolcEngineClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("volcengine api_key is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
	}

	model := cfg.Model
	if model == "" {
		model = "doubao-embedding-vision-250615"
	}

	return &VolcEngineClient{
		apiKey:     cfg.APIKey,
		endpoint:   endpoint,
		model:      model,
		dimensions: cfg.Dimensions,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Embed generates an embedding for a single text
func (c *VolcEngineClient) Embed(ctx context.Context, text string) ([]float32, error) {
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
func (c *VolcEngineClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// The multimodal embeddings endpoint accepts a single sample per request.
	// Loop to support batch semantics.
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		vector, err := c.embedText(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings[i] = vector
	}

	return embeddings, nil
}

func (c *VolcEngineClient) embedText(ctx context.Context, text string) ([]float32, error) {
	// Build request inputs for a single sample
	inputs := []VolcEngineInput{{
		Type: "text",
		Text: text,
	}}

	req := VolcEngineEmbeddingRequest{
		Input:          inputs,
		Model:          c.model,
		EncodingFormat: "float",
		Dimensions:     c.dimensions,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytesReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Send request
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

	// Parse response
	var apiResp VolcEngineEmbeddingResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, err := parseVolcEngineEmbeddingData(apiResp.Data)
	if err != nil {
		return nil, err
	}
	if len(data) != 1 {
		return nil, fmt.Errorf("expected 1 embedding, got %d", len(data))
	}

	return data[0].Embedding, nil
}

// Dimensions returns the dimension of the embeddings
func (c *VolcEngineClient) Dimensions() int {
	return c.dimensions
}

// bytesReader creates an io.Reader from a byte slice
func bytesReader(b []byte) io.Reader {
	return &byteReader{b: b}
}

// byteReader is a simple io.Reader implementation for []byte
type byteReader struct {
	b []byte
}

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

func parseVolcEngineEmbeddingData(raw json.RawMessage) ([]VolcEngineEmbeddingData, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty embedding data")
	}

	// Trim leading whitespace to detect JSON type.
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			continue
		case '[':
			var data []VolcEngineEmbeddingData
			if err := json.Unmarshal(raw, &data); err != nil {
				return nil, fmt.Errorf("failed to parse embedding array: %w", err)
			}
			return data, nil
		case '{':
			var data VolcEngineEmbeddingData
			if err := json.Unmarshal(raw, &data); err != nil {
				return nil, fmt.Errorf("failed to parse embedding object: %w", err)
			}
			return []VolcEngineEmbeddingData{data}, nil
		default:
			return nil, fmt.Errorf("unexpected embedding data format")
		}
	}

	return nil, fmt.Errorf("empty embedding data")
}
