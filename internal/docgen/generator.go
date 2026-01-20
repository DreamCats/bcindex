package docgen

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
			Timeout: 120 * time.Second,
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
				"content": "You are a Go documentation expert. Generate concise, clear, and useful doc comments following Go conventions.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.2,
		"top_p":       0.7,
		"max_tokens":  8192, // Handle batches of 10 symbols
		"stream":      true,  // Enable streaming
	}

	// Try to use JSON mode if supported
	reqBody["response_format"] = map[string]string{"type": "json_object"}
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
	httpReq.Header.Set("Accept", "text/event-stream")

	// Send request
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read streaming response
	var content strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue // Empty line between SSE chunks
		}

		// SSE format: data: {...}
		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
				Index        int `json:"index"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			// Skip invalid JSON lines
			continue
		}

		// Check for finish reason
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != nil {
			if *chunk.Choices[0].FinishReason == "stop" {
				break
			}
		}

		// Append content
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	contentStr := content.String()

	// Validate we got content
	if contentStr == "" {
		return nil, fmt.Errorf("empty content from streaming response")
	}

	// Parse the JSON content
	var batchResp GenerateBatchResponse
	if err := json.Unmarshal([]byte(contentStr), &batchResp); err != nil {
		// Try to fix common JSON issues
		fixed, fixErr := fixJSON(contentStr)
		if fixErr != nil {
			return nil, fmt.Errorf("failed to parse response content as JSON: %w, original content: %s", fixErr, contentStr)
		}
		if err := json.Unmarshal([]byte(fixed), &batchResp); err != nil {
			return nil, fmt.Errorf("failed to parse fixed JSON: %w, fixed content: %s", err, fixed)
		}
	}

	// Validate we got the expected number of results
	if len(batchResp.Items) != len(symbols) {
		// If we got more items than requested, truncate
		if len(batchResp.Items) > len(symbols) {
			batchResp.Items = batchResp.Items[:len(symbols)]
		} else if len(batchResp.Items) == 0 {
			return nil, fmt.Errorf("no items in response, content: %s", contentStr)
		}
	}

	return batchResp.Items, nil
}

// buildPrompt constructs the prompt for documentation generation
func (g *Generator) buildPrompt(symbols []SymbolInfo) string {
	var prompt strings.Builder

	prompt.WriteString("You are a Go documentation expert. Generate doc comments for the following Go symbols.\n\n")
	prompt.WriteString(fmt.Sprintf("You will generate documentation for %d symbols.\n\n", len(symbols)))

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. First sentence MUST start with the symbol name (e.g., 'Foo creates...', 'Bar represents...')\n")
	prompt.WriteString("2. Be concise: one sentence summary + optional key constraints/errors/side effects\n")
	prompt.WriteString("3. Use Chinese for explanations + English for technical terms\n")
	prompt.WriteString("4. Focus on WHAT and WHY, not HOW (no implementation details)\n")
	prompt.WriteString(fmt.Sprintf("5. Return exactly %d items in the JSON response\n\n", len(symbols)))

	prompt.WriteString("OUTPUT FORMAT (strict JSON):\n")
	prompt.WriteString("```json\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"items\": [\n")
	prompt.WriteString("    {\"id\": \"<symbol-id>\", \"comment\": \"<doc comment>\"},\n")
	prompt.WriteString("    ...\n")
	prompt.WriteString("  ]\n")
	prompt.WriteString("}\n")
	prompt.WriteString("```\n\n")

	prompt.WriteString("GUIDELINES per symbol type:\n")
	prompt.WriteString("- func/method: '<Name> <verb(s)>... <object/purpose>.\\n<Key constraints, errors, or side effects if any>'\n")
	prompt.WriteString("- struct: '<Name> represents/holds <role/responsibility>.\\n<Key fields or invariants if important>'\n")
	prompt.WriteString("- interface: '<Name> defines <contract/behavior>.\\n<Key methods or implementation requirements>'\n")
	prompt.WriteString("- type: '<Name> is <type alias/definition>.\\n<Purpose or usage context>'\n\n")

	prompt.WriteString("SYMBOLS TO DOCUMENT:\n\n")

	for i, sym := range symbols {
		prompt.WriteString(fmt.Sprintf("[%d] ID: %s\n", i+1, sym.ID))
		prompt.WriteString(fmt.Sprintf("    Name: %s\n", sym.Name))
		prompt.WriteString(fmt.Sprintf("    Kind: %s\n", sym.Kind))
		prompt.WriteString(fmt.Sprintf("    Package: %s\n", sym.Package))
		prompt.WriteString(fmt.Sprintf("    Signature: %s\n", sym.Signature))
		if sym.Receiver != "" {
			prompt.WriteString(fmt.Sprintf("    Receiver: %s\n", sym.Receiver))
		}
		if sym.FilePath != "" {
			prompt.WriteString(fmt.Sprintf("    File: %s:%d\n", sym.FilePath, sym.Line))
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString(fmt.Sprintf("\nGenerate JSON with exactly %d items now:\n", len(symbols)))

	return prompt.String()
}

// fixJSON attempts to fix common JSON formatting issues
func fixJSON(s string) (string, error) {
	// Remove markdown code blocks if present
	if len(s) > 7 && s[0:7] == "```json" {
		end := findJSONEnd(s)
		if end > 0 {
			s = s[7:end]
		}
	} else if len(s) > 3 && s[0:3] == "```" {
		end := findJSONEnd(s)
		if end > 0 {
			s = s[3:end]
		}
	}

	// Fix common escape sequence issues
	// Replace invalid escape sequences like \  (backslash followed by space) with proper escaping
	s = strings.ReplaceAll(s, `\\ `, ` `)
	// Fix double backslashes in strings that should be single
	s = strings.ReplaceAll(s, `\\\\n`, `\n`)
	s = strings.ReplaceAll(s, `\\\\t`, `\t`)
	s = strings.ReplaceAll(s, `\\\"`, `\"`)

	return strings.TrimSpace(s), nil
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
