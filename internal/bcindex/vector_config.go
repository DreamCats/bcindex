package bcindex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

type VectorConfig struct {
	QdrantURL        string `yaml:"qdrant_url"`
	QdrantPath       string `yaml:"qdrant_path"`
	QdrantAPIKey     string `yaml:"qdrant_api_key"`
	QdrantCollection string `yaml:"qdrant_collection"`
	QdrantBin        string `yaml:"qdrant_bin"`
	QdrantHTTPPort   int    `yaml:"qdrant_http_port"`
	QdrantGRPCPort   int    `yaml:"qdrant_grpc_port"`
	QdrantAutoStart  bool   `yaml:"qdrant_auto_start"`

	VolcesEndpoint     string        `yaml:"volces_endpoint"`
	VolcesAPIKey       string        `yaml:"volces_api_key"`
	VolcesModel        string        `yaml:"volces_model"`
	VolcesDimensions   int           `yaml:"volces_dimensions"`
	VolcesEncoding     string        `yaml:"volces_encoding"`
	VolcesInstructions string        `yaml:"volces_instructions"`
	VolcesTimeout      time.Duration `yaml:"volces_timeout"`

	VectorEnabled   bool `yaml:"vector_enabled"`
	VectorBatchSize int  `yaml:"vector_batch_size"`
	VectorMaxChars  int  `yaml:"vector_max_chars"`
	VectorWorkers   int  `yaml:"vector_workers"`
	VectorRerankTop int  `yaml:"vector_rerank_candidates"`
	VectorOverlap   int  `yaml:"vector_overlap_chars"`
	QueryTopK       int  `yaml:"query_top_k"`
}

var ErrVectorConfigNotFound = errors.New("vector config not found")

type vectorConfigInit struct {
	QdrantPath       string `yaml:"qdrant_path"`
	QdrantCollection string `yaml:"qdrant_collection"`
	VolcesEndpoint   string `yaml:"volces_endpoint"`
	VolcesAPIKey     string `yaml:"volces_api_key"`
	VolcesModel      string `yaml:"volces_model"`
	VectorEnabled    bool   `yaml:"vector_enabled"`
	QueryTopK        int    `yaml:"query_top_k"`
	ExcludeDirs      []string `yaml:"exclude_dirs"`
	Exclude          []string `yaml:"exclude"`
	UseGitignore     bool     `yaml:"use_gitignore"`
}

type appConfigInit struct {
	vectorConfigInit `yaml:",inline"`
	Index            IndexConfig `yaml:"index"`
	Query            QueryConfig `yaml:"query"`
}

func LoadVectorConfig() (VectorConfig, error) {
	path, err := vectorConfigPath()
	if err != nil {
		return VectorConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return VectorConfig{}, fmt.Errorf("%w: %s", ErrVectorConfigNotFound, path)
		}
		return VectorConfig{}, err
	}
	cfg := defaultVectorConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return VectorConfig{}, fmt.Errorf("parse vector config: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func LoadVectorConfigOptional() (VectorConfig, bool, error) {
	cfg, err := LoadVectorConfig()
	if err == nil {
		return cfg, true, nil
	}
	if errors.Is(err, ErrVectorConfigNotFound) {
		return VectorConfig{}, false, nil
	}
	return VectorConfig{}, false, err
}

func defaultVectorConfig() VectorConfig {
	return VectorConfig{
		QdrantURL:        "http://127.0.0.1:6333",
		QdrantPath:       defaultQdrantPath(),
		QdrantCollection: "bcindex_vectors",
		QdrantHTTPPort:   6333,
		QdrantGRPCPort:   6334,
		QdrantAutoStart:  true,
		VolcesEndpoint:   "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal",
		VolcesDimensions: 1024,
		VolcesEncoding:   "float",
		VolcesTimeout:    30 * time.Second,
		VectorEnabled:    true,
		VectorBatchSize:  8,
		VectorMaxChars:   1500,
		VectorWorkers:    defaultVectorWorkers(),
		VectorRerankTop:  300,
		VectorOverlap:    80,
		QueryTopK:        10,
	}
}

func (c *VectorConfig) applyDefaults() {
	c.QdrantPath = expandUserPath(c.QdrantPath)
	if c.QdrantHTTPPort == 0 {
		c.QdrantHTTPPort = 6333
	}
	if c.QdrantGRPCPort == 0 {
		c.QdrantGRPCPort = 6334
	}
	if c.QdrantURL == "" {
		c.QdrantURL = fmt.Sprintf("http://127.0.0.1:%d", c.QdrantHTTPPort)
	}
	if c.QdrantCollection == "" {
		c.QdrantCollection = "bcindex_vectors"
	}
	if c.VolcesEndpoint == "" {
		c.VolcesEndpoint = "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
	}
	if c.VolcesDimensions == 0 {
		c.VolcesDimensions = 1024
	}
	if c.VolcesEncoding == "" {
		c.VolcesEncoding = "float"
	}
	if c.VolcesTimeout == 0 {
		c.VolcesTimeout = 30 * time.Second
	}
	if c.VectorBatchSize == 0 {
		c.VectorBatchSize = 8
	}
	if c.VectorMaxChars == 0 {
		c.VectorMaxChars = 1500
	}
	if c.VectorWorkers == 0 {
		c.VectorWorkers = defaultVectorWorkers()
	}
	if c.VectorRerankTop == 0 {
		c.VectorRerankTop = 300
	}
	if c.VectorOverlap == 0 {
		c.VectorOverlap = 80
	}
	if c.QueryTopK == 0 {
		c.QueryTopK = 10
	}
}

func defaultVectorConfigInit() vectorConfigInit {
	defaultCfg := defaultIndexConfig()
	return vectorConfigInit{
		QdrantPath:       defaultQdrantPath(),
		QdrantCollection: "bcindex_vectors",
		VolcesEndpoint:   "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal",
		VolcesAPIKey:     "",
		VolcesModel:      "",
		VectorEnabled:    true,
		QueryTopK:        10,
		ExcludeDirs:      defaultCfg.ExcludeDirs,
		Exclude:          defaultCfg.Exclude,
		UseGitignore:     defaultCfg.UseGitignore,
	}
}

func defaultQdrantPath() string {
	return "~/.bcindex/qdrant"
}

func defaultVectorWorkers() int {
	workers := runtime.NumCPU()
	if workers > 8 {
		return 8
	}
	if workers < 1 {
		return 1
	}
	return workers
}

func expandUserPath(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
		return filepath.Join(home, path[2:])
	}
	return path
}

func vectorConfigPath() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config", "bcindex.yaml"), nil
}

func WriteDefaultVectorConfig() (string, error) {
	path, err := vectorConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	cfg := appConfigInit{
		vectorConfigInit: defaultVectorConfigInit(),
		Index:            defaultIndexConfig(),
		Query:            defaultQueryConfig(),
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
