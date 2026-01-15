package bcindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type VolcesEmbeddingClient struct {
	endpoint     string
	apiKey       string
	model        string
	dimensions   int
	encoding     string
	instructions string
	client       *http.Client
}

func NewVolcesEmbeddingClient(cfg VectorConfig) (*VolcesEmbeddingClient, error) {
	if strings.TrimSpace(cfg.VolcesAPIKey) == "" {
		return nil, fmt.Errorf("volces api key is required")
	}
	if strings.TrimSpace(cfg.VolcesModel) == "" {
		return nil, fmt.Errorf("volces model is required")
	}
	timeout := cfg.VolcesTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &VolcesEmbeddingClient{
		endpoint:     cfg.VolcesEndpoint,
		apiKey:       cfg.VolcesAPIKey,
		model:        cfg.VolcesModel,
		dimensions:   cfg.VolcesDimensions,
		encoding:     cfg.VolcesEncoding,
		instructions: cfg.VolcesInstructions,
		client:       &http.Client{Timeout: timeout},
	}, nil
}

func (c *VolcesEmbeddingClient) EmbedTexts(ctx context.Context, texts []string) ([]EmbeddingResult, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}
	inputs := make([]map[string]any, 0, len(texts))
	for _, text := range texts {
		inputs = append(inputs, map[string]any{
			"type": "text",
			"text": text,
		})
	}
	reqBody := map[string]any{
		"model": c.model,
		"input": inputs,
	}
	if strings.TrimSpace(c.encoding) != "" {
		reqBody["encoding_format"] = c.encoding
	}
	if c.dimensions > 0 {
		reqBody["dimensions"] = c.dimensions
	}
	if strings.TrimSpace(c.instructions) != "" {
		reqBody["instructions"] = c.instructions
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("volces status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed volcesEmbeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data) == 0 {
		if parsed.Message != "" {
			return nil, fmt.Errorf("volces embedding response missing data: %s", parsed.Message)
		}
		if parsed.Error != nil {
			return nil, fmt.Errorf("volces embedding response missing data")
		}
		return nil, fmt.Errorf("volces embedding response empty")
	}
	items, err := parseVolcesEmbeddingData(parsed.Data)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("volces embedding response empty")
	}
	results := make([]EmbeddingResult, 0, len(items))
	for idx, item := range items {
		if item.Index == 0 && len(items) == 1 {
			item.Index = idx
		}
		vec := make([]float32, len(item.Embedding))
		for i, val := range item.Embedding {
			vec[i] = float32(val)
		}
		results = append(results, EmbeddingResult{
			Index:  item.Index,
			Vector: vec,
		})
	}
	return results, nil
}

type volcesEmbeddingResponse struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Error   any             `json:"error"`
}

type volcesEmbeddingItem struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
	Object    string    `json:"object"`
}

func parseVolcesEmbeddingData(raw json.RawMessage) ([]volcesEmbeddingItem, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []volcesEmbeddingItem
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var single volcesEmbeddingItem
	if err := json.Unmarshal(raw, &single); err == nil {
		if len(single.Embedding) > 0 {
			return []volcesEmbeddingItem{single}, nil
		}
	}
	var wrapper struct {
		Data []volcesEmbeddingItem `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if len(wrapper.Data) > 0 {
			return wrapper.Data, nil
		}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		if embedded, ok := obj["embedding"]; ok {
			var vec []float64
			if err := json.Unmarshal(embedded, &vec); err == nil {
				return []volcesEmbeddingItem{{Embedding: vec, Index: 0, Object: "embedding"}}, nil
			}
		}
		if nested, ok := obj["data"]; ok {
			return parseVolcesEmbeddingData(nested)
		}
	}
	return nil, fmt.Errorf("volces embedding response parse failed")
}
