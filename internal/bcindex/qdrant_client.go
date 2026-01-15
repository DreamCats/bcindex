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

type QdrantClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewQdrantClient(url, apiKey string) *QdrantClient {
	return &QdrantClient{
		baseURL: strings.TrimRight(url, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

func NewQdrantClientFromConfig(ctx context.Context, cfg VectorConfig) (*QdrantClient, func(), error) {
	proc, err := EnsureQdrantRunning(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	client := NewQdrantClient(cfg.QdrantURL, cfg.QdrantAPIKey)
	cleanup := func() {
		if proc != nil {
			proc.Stop()
		}
	}
	return client, cleanup, nil
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error {
	if distance == "" {
		distance = "Cosine"
	}
	_, err := c.doRequest(ctx, http.MethodGet, "/collections/"+name, nil)
	if err == nil {
		return nil
	}
	req := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": distance,
		},
	}
	_, err = c.doRequest(ctx, http.MethodPut, "/collections/"+name, req)
	return err
}

func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []VectorPoint) error {
	if len(points) == 0 {
		return nil
	}
	payload := make([]map[string]any, 0, len(points))
	for _, p := range points {
		payload = append(payload, map[string]any{
			"id":      p.ID,
			"vector":  p.Vector,
			"payload": p.Payload,
		})
	}
	req := map[string]any{"points": payload}
	_, err := c.doRequest(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", req)
	return err
}

func (c *QdrantClient) DeletePointsByIDs(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	req := map[string]any{
		"points": ids,
	}
	_, err := c.doRequest(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", req)
	return err
}

func (c *QdrantClient) DeletePointsByFilter(ctx context.Context, collection string, filter map[string]any) error {
	if len(filter) == 0 {
		return nil
	}
	req := map[string]any{
		"filter": filter,
	}
	_, err := c.doRequest(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", req)
	return err
}

type QdrantSearchPoint struct {
	ID      string
	Score   float64
	Payload map[string]any
}

func (c *QdrantClient) SearchPoints(ctx context.Context, collection string, vector []float32, limit int, filter map[string]any) ([]QdrantSearchPoint, error) {
	if limit <= 0 {
		limit = 10
	}
	req := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if len(filter) > 0 {
		req["filter"] = filter
	}
	data, err := c.doRequest(ctx, http.MethodPost, "/collections/"+collection+"/points/search", req)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	points := make([]QdrantSearchPoint, 0, len(parsed.Result))
	for _, item := range parsed.Result {
		points = append(points, QdrantSearchPoint{
			ID:      fmt.Sprintf("%v", item.ID),
			Score:   item.Score,
			Payload: item.Payload,
		})
	}
	return points, nil
}

func (c *QdrantClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var buf io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qdrant status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func qdrantMatchFilter(key string, value any) map[string]any {
	return map[string]any{
		"key": key,
		"match": map[string]any{
			"value": value,
		},
	}
}

func qdrantMustFilter(conditions ...map[string]any) map[string]any {
	return map[string]any{
		"must": conditions,
	}
}
