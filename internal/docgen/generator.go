package docgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DreamCats/bcindex/internal/config"
)

// Generator generates documentation for Go symbols using LLM
type Generator struct {
	cfg      *config.DocGenConfig
	client   *http.Client
	apiKey   string
	endpoint string
	model    string
}

// NewGenerator creates a new documentation generator
func NewGenerator(cfg *config.DocGenConfig) (*Generator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("docgen config is required")
	}

	// Use embedding API key if docgen api_key is not set
	apiKey := cfg.APIKey
	if apiKey == "" {
		return nil, fmt.Errorf("docgen.api_key is required (or configure embedding.api_key)")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
	}

	model := cfg.Model
	if model == "" {
		model = "doubao-1-5-pro-32k-250115"
	}

	return &Generator{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
	}, nil
}

// SymbolInfo represents information about a symbol for documentation generation
type SymbolInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Package   string `json:"package"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	Receiver  string `json:"receiver,omitempty"`
	Existing  string `json:"existing,omitempty"` // Existing doc comment, if any
}

// GenerateResult is the result of documentation generation
type GenerateResult struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
	Error   string `json:"error,omitempty"`
}

// GenerateBatchRequest is the request for batch documentation generation
type GenerateBatchRequest struct {
	Symbols []SymbolInfo `json:"symbols"`
}

// GenerateBatchResponse is the response from LLM
type GenerateBatchResponse struct {
	Items []GenerateResult `json:"items"`
}

// Generate generates documentation for a single symbol
func (g *Generator) Generate(ctx context.Context, symbol SymbolInfo) (string, error) {
	results, err := g.GenerateBatch(ctx, []SymbolInfo{symbol})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no result returned")
	}
	if results[0].Error != "" {
		return "", fmt.Errorf("LLM error: %s", results[0].Error)
	}
	return results[0].Comment, nil
}

// GenerateBatch generates documentation for multiple symbols
func (g *Generator) GenerateBatch(ctx context.Context, symbols []SymbolInfo) ([]GenerateResult, error) {
	if len(symbols) == 0 {
		return nil, nil
	}

	// Build the prompt
	prompt := g.buildPrompt(symbols)

	// Build request body
	reqBody := map[string]interface{}{
		"model": g.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You are a Go code documentation expert. Generate concise, clear, and useful doc comments following Go conventions.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.2,
		"top_p":       0.7,
		"max_tokens":  2048,
	}

	// Try to use JSON mode if supported
	reqBody["response_format"] = map[string]string{"type": "json_object"}
	reqBody["stream"] = false
	reqBody["thinking"] = map[string]interface{}{"type": "disabled"}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", g.endpoint, io.NopCloser(NewReader(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.apiKey)

	// Send request
	resp, err := g.client.Do(httpReq)
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
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from API")
	}

	content := apiResp.Choices[0].Message.Content

	// Parse the JSON content
	var batchResp GenerateBatchResponse
	if err := json.Unmarshal([]byte(content), &batchResp); err != nil {
		// Try to fix common JSON issues
		fixed, fixErr := fixJSON(content)
		if fixErr != nil {
			return nil, fmt.Errorf("failed to parse response content as JSON: %w", fixErr)
		}
		if err := json.Unmarshal([]byte(fixed), &batchResp); err != nil {
			return nil, fmt.Errorf("failed to parse fixed JSON: %w", err)
		}
	}

	return batchResp.Items, nil
}

// buildPrompt constructs the prompt for documentation generation
func (g *Generator) buildPrompt(symbols []SymbolInfo) string {
	var prompt string

	prompt += "Generate Go doc comments for the following symbols.\n\n"
	prompt += "IMPORTANT REQUIREMENTS:\n"
	prompt += "1. Start each comment with the symbol name (e.g., 'Foo does ...')\n"
	prompt += "2. Keep it concise: one sentence summary + optional key constraints/errors\n"
	prompt += "3. Use Chinese for explanation + English for technical terms\n"
	prompt += "4. Don't include implementation details\n"
	prompt += "5. Output valid JSON only\n\n"

	prompt += `Output format (JSON):
{
  "items": [
    {"id": "symbol-id", "comment": "SymbolName does ...\\nAdditional info if needed."}
  ]
}

`
	prompt += "Symbols to document:\n\n"

	for _, sym := range symbols {
		prompt += fmt.Sprintf("--- Symbol %s ---\n", sym.ID)
		prompt += fmt.Sprintf("Name: %s\n", sym.Name)
		prompt += fmt.Sprintf("Kind: %s\n", sym.Kind)
		prompt += fmt.Sprintf("Package: %s\n", sym.Package)
		prompt += fmt.Sprintf("Signature: %s\n", sym.Signature)
		if sym.Receiver != "" {
			prompt += fmt.Sprintf("Receiver: %s\n", sym.Receiver)
		}
		prompt += "\n"
	}

	return prompt
}

// fixJSON attempts to fix common JSON formatting issues
func fixJSON(s string) (string, error) {
	// Remove markdown code blocks if present
	if len(s) > 7 && s[0:7] == "```json" {
		end := findJSONEnd(s)
		if end > 0 {
			return s[7:end], nil
		}
	}
	if len(s) > 3 && s[0:3] == "```" {
		end := findJSONEnd(s)
		if end > 0 {
			return s[3:end], nil
		}
	}
	return s, nil
}

func findJSONEnd(s string) int {
	for i := 3; i < len(s)-3; i++ {
		if s[i:i+3] == "```" {
			return i
		}
	}
	return -1
}

// Reader is a simple io.Reader implementation for []byte
type Reader struct {
	b []byte
}

func NewReader(b []byte) *Reader {
	return &Reader{b: b}
}

func (r *Reader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
