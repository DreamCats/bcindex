package bcindex

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type QueryConfig struct {
	MaxContextChars int `yaml:"max_context_chars"`
}

func defaultQueryConfig() QueryConfig {
	return QueryConfig{
		MaxContextChars: 20000,
	}
}

func LoadQueryConfigOptional() (QueryConfig, bool, error) {
	path, err := vectorConfigPath()
	if err != nil {
		return defaultQueryConfig(), false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultQueryConfig(), false, nil
		}
		return defaultQueryConfig(), false, err
	}
	var wrapper struct {
		Query           QueryConfig `yaml:"query"`
		MaxContextChars int         `yaml:"max_context_chars"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return defaultQueryConfig(), true, fmt.Errorf("parse query config: %w", err)
	}
	cfg := wrapper.Query
	if cfg.MaxContextChars == 0 {
		cfg.MaxContextChars = wrapper.MaxContextChars
	}
	if cfg.MaxContextChars == 0 {
		cfg.MaxContextChars = defaultQueryConfig().MaxContextChars
	}
	return cfg, true, nil
}
