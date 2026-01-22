package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	// Deprecated: Repo path is now determined by current working directory
	// This field is kept for backward compatibility but is no longer used
	Repo      RepoConfig      `yaml:"repo,omitempty"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Database  DatabaseConfig  `yaml:"database"`
	Indexer   IndexerConfig   `yaml:"indexer,omitempty"`
	Search    SearchConfig    `yaml:"search,omitempty"`
	Evidence  EvidenceConfig  `yaml:"evidence,omitempty"`
	DocGen    DocGenConfig    `yaml:"docgen,omitempty"`
}

// RepoConfig holds repository-specific configuration (deprecated)
type RepoConfig struct {
	Path string `yaml:"path,omitempty"`
}

// EmbeddingConfig holds embedding service configuration
type EmbeddingConfig struct {
	Provider string `yaml:"provider"` // "volcengine" | "openai" | "local"

	// VolcEngine specific
	APIKey   string `yaml:"api_key"`
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`

	// OpenAI specific (for future use)
	OpenAIAPIKey string `yaml:"openai_api_key,omitempty"`
	OpenAIModel  string `yaml:"openai_model,omitempty"`

	// Embedding parameters
	Dimensions     int    `yaml:"dimensions"`      // 1024 | 2048
	BatchSize      int    `yaml:"batch_size"`      // Batch size for embedding
	EncodingFormat string `yaml:"encoding_format"` // "float" | "base64"
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	// Path to SQLite database file
	// If empty, uses ~/.bcindex/data/<repo-name>.db
	Path string `yaml:"path,omitempty"`
}

// IndexerConfig holds indexer-specific configuration
type IndexerConfig struct {
	MaxWorkers int      `yaml:"max_workers,omitempty"` // Maximum number of goroutines
	SkipTests  bool     `yaml:"skip_tests,omitempty"`  // Skip test files
	Exclude    []string `yaml:"exclude,omitempty"`     // Exclude patterns
}

// SearchConfig holds search-specific configuration
type SearchConfig struct {
	DefaultTopK     int     `yaml:"default_top_k,omitempty"`     // Default number of results
	VectorWeight    float32 `yaml:"vector_weight,omitempty"`     // Vector search weight (0-1)
	KeywordWeight   float32 `yaml:"keyword_weight,omitempty"`    // Keyword search weight (0-1)
	GraphWeight     float32 `yaml:"graph_weight,omitempty"`      // Graph ranking weight (0-1)
	EnableGraphRank bool    `yaml:"enable_graph_rank,omitempty"` // Enable graph-based ranking
	SynonymsFile    string  `yaml:"synonyms_file,omitempty"`     // Repo-relative synonyms file
}

// EvidenceConfig holds evidence pack configuration
type EvidenceConfig struct {
	MaxPackages int `yaml:"max_packages,omitempty"` // Maximum packages in evidence pack
	MaxSymbols  int `yaml:"max_symbols,omitempty"`  // Maximum symbols in evidence pack
	MaxSnippets int `yaml:"max_snippets,omitempty"` // Maximum code snippets
	MaxLines    int `yaml:"max_lines,omitempty"`    // Maximum total lines across snippets
}

// DocGenConfig holds docgen (documentation generator) configuration
type DocGenConfig struct {
	Provider string `yaml:"provider,omitempty"` // "volcengine" | "openai"
	APIKey   string `yaml:"api_key,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	Model    string `yaml:"model,omitempty"`
}

// Load loads configuration from the default config file
// Default location: ~/.bcindex/config/bcindex.yaml
func Load() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".bcindex", "config", "bcindex.yaml")
	return LoadFromFile(configPath)
}

// LoadFromFile loads configuration from a specific file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			homeDir, _ := os.UserHomeDir()
			defaultPath := filepath.Join(homeDir, ".bcindex", "config", "bcindex.yaml")
			return nil, &ConfigNotFoundError{
				RequestedPath: path,
				DefaultPath:   defaultPath,
			}
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	if err := cfg.applyDefaults(); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// ConfigNotFoundError is returned when config file is not found
type ConfigNotFoundError struct {
	RequestedPath string
	DefaultPath   string
}

func (e *ConfigNotFoundError) Error() string {
	return fmt.Sprintf("config file not found at: %s\n\nDefault location: %s\n\nYou can:\n"+
		"  1. Create the config file at the default location\n"+
		"  2. Specify a custom path with -config flag\n"+
		"  3. See 'bcindex init' for help creating a config file",
		e.RequestedPath, e.DefaultPath)
}

// IsConfigNotFound checks if error is config not found
func IsConfigNotFound(err error) bool {
	_, ok := err.(*ConfigNotFoundError)
	return ok
}

// expandPath expands ~ and $HOME to the user's home directory
// Supports both:
//
//	~/.bcindex/data/bcindex.db
//	$HOME/.bcindex/data/bcindex.db
func expandPath(path string) string {
	// Handle $HOME environment variable
	if strings.HasPrefix(path, "$HOME/") || path == "$HOME" {
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			// Fallback to UserHomeDir if HOME is not set
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				// If we can't get home dir, return path as-is
				return path
			}
		}
		if path == "$HOME" {
			return homeDir
		}
		return filepath.Join(homeDir, path[6:])
	}

	// Handle ~ shorthand
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// If we can't get home dir, return path as-is
			return path
		}
		if path == "~" {
			return homeDir
		}
		return filepath.Join(homeDir, path[2:])
	}

	return path
}

// applyDefaults sets default values for missing configuration
func (c *Config) applyDefaults() error {
	// Set default embedding provider
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "volcengine"
	}

	// Set default model
	if c.Embedding.Model == "" {
		c.Embedding.Model = "doubao-embedding-vision-250615"
	}

	// Set default dimensions
	if c.Embedding.Dimensions == 0 {
		c.Embedding.Dimensions = 2048
	}

	// Set default batch size
	if c.Embedding.BatchSize == 0 {
		c.Embedding.BatchSize = 10
	}

	// Set default encoding format
	if c.Embedding.EncodingFormat == "" {
		c.Embedding.EncodingFormat = "float"
	}

	// Expand ~ in database path
	if c.Database.Path != "" {
		c.Database.Path = expandPath(c.Database.Path)
	}

	// Set default database path (if not specified, will be determined per-repo)
	// Leave empty if user wants per-repo databases
	// Otherwise use the default global database
	// Note: Database.Path is now optional - if empty, indexer will use repo-specific path

	// Set default indexer options
	if c.Indexer.SkipTests {
		// already set
	}
	if c.Indexer.MaxWorkers == 0 {
		c.Indexer.MaxWorkers = 4
	}

	// Set default search options
	if c.Search.DefaultTopK == 0 {
		c.Search.DefaultTopK = 10
	}
	if c.Search.VectorWeight == 0 && c.Search.KeywordWeight == 0 && c.Search.GraphWeight == 0 {
		c.Search.VectorWeight = 0.6
		c.Search.KeywordWeight = 0.2
		c.Search.GraphWeight = 0.2
	}
	c.Search.EnableGraphRank = true // default enabled
	if c.Search.SynonymsFile == "" {
		c.Search.SynonymsFile = "domain_aliases.yaml"
	}

	// Set default evidence options
	if c.Evidence.MaxPackages == 0 {
		c.Evidence.MaxPackages = 3
	}
	if c.Evidence.MaxSymbols == 0 {
		c.Evidence.MaxSymbols = 10
	}
	if c.Evidence.MaxSnippets == 0 {
		c.Evidence.MaxSnippets = 5
	}
	if c.Evidence.MaxLines == 0 {
		c.Evidence.MaxLines = 200
	}

	// Set default docgen options
	if c.DocGen.Provider == "" {
		c.DocGen.Provider = "volcengine"
	}
	if c.DocGen.Endpoint == "" {
		c.DocGen.Endpoint = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
	}
	if c.DocGen.Model == "" {
		c.DocGen.Model = "doubao-1-5-pro-32k-250115"
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate embedding configuration based on provider
	switch c.Embedding.Provider {
	case "volcengine":
		if c.Embedding.APIKey == "" {
			return fmt.Errorf("volcengine provider requires api_key")
		}
	case "openai":
		if c.Embedding.OpenAIAPIKey == "" {
			return fmt.Errorf("openai provider requires openai_api_key")
		}
	default:
		return fmt.Errorf("unsupported embedding provider: %s", c.Embedding.Provider)
	}

	// Validate dimensions
	if c.Embedding.Dimensions != 1024 && c.Embedding.Dimensions != 2048 {
		return fmt.Errorf("dimensions must be 1024 or 2048, got: %d", c.Embedding.Dimensions)
	}

	// Validate batch size
	if c.Embedding.BatchSize <= 0 || c.Embedding.BatchSize > 100 {
		return fmt.Errorf("batch_size must be between 1 and 100, got: %d", c.Embedding.BatchSize)
	}

	return nil
}

// Save saves the configuration to the default location
func (c *Config) Save() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".bcindex", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "bcindex.yaml")
	return c.SaveToFile(configPath)
}

// SaveToFile saves the configuration to a specific file
func (c *Config) SaveToFile(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

const defaultConfigTemplate = `# BCIndex Configuration
#
# Copy and edit this file for your environment.
# Default location: $HOME/.bcindex/config/bcindex.yaml

embedding:
  # Provider: "volcengine" or "openai"
  provider: volcengine

  # VolcEngine configuration
  api_key: your-volcengine-api-key
  endpoint: https://ark.cn-beijing.volces.com/api/v3
  model: doubao-embedding-vision-250615
  dimensions: 2048
  batch_size: 10
  encoding_format: float

  # OpenAI configuration (alternative)
  # provider: openai
  # openai_api_key: your-openai-api-key
  # openai_model: text-embedding-3-small
  # dimensions: 1536
  # batch_size: 100
  # encoding_format: float
`

// WriteDefaultTemplate creates a default configuration file if it does not exist.
// It returns true if a file was created, false if it already existed.
func WriteDefaultTemplate(path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("config path is empty")
	}
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to stat config file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(defaultConfigTemplate), 0644); err != nil {
		return false, fmt.Errorf("failed to write config template: %w", err)
	}

	return true, nil
}
