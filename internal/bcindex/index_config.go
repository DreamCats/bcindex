package bcindex

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type IndexTier string

const (
	IndexTierFast     IndexTier = "fast"
	IndexTierBalanced IndexTier = "balanced"
	IndexTierFull     IndexTier = "full"
)

type IndexConfig struct {
	Tier string `yaml:"tier"`
}

type IndexOptions struct {
	Tier IndexTier
}

func defaultIndexConfig() IndexConfig {
	return IndexConfig{Tier: string(IndexTierFast)}
}

func (c *IndexConfig) applyDefaults() error {
	if strings.TrimSpace(c.Tier) == "" {
		c.Tier = string(IndexTierFast)
		return nil
	}
	tier, err := ParseIndexTier(c.Tier)
	if err != nil {
		return err
	}
	c.Tier = string(tier)
	return nil
}

func ParseIndexTier(value string) (IndexTier, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return IndexTierFast, nil
	}
	switch v {
	case string(IndexTierFast), string(IndexTierBalanced), string(IndexTierFull):
		return IndexTier(v), nil
	default:
		return "", fmt.Errorf("unknown index tier: %s", value)
	}
}

func LoadIndexConfigOptional() (IndexConfig, bool, error) {
	path, err := vectorConfigPath()
	if err != nil {
		return defaultIndexConfig(), false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultIndexConfig(), false, nil
		}
		return defaultIndexConfig(), false, err
	}
	var wrapper struct {
		Index IndexConfig `yaml:"index"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return defaultIndexConfig(), true, fmt.Errorf("parse index config: %w", err)
	}
	cfg := wrapper.Index
	if err := cfg.applyDefaults(); err != nil {
		return defaultIndexConfig(), true, err
	}
	return cfg, true, nil
}

func ResolveIndexTier(cliValue string) (IndexTier, error) {
	if strings.TrimSpace(cliValue) != "" {
		return ParseIndexTier(cliValue)
	}
	cfg, _, err := LoadIndexConfigOptional()
	if err != nil {
		return "", err
	}
	return IndexTier(cfg.Tier), nil
}

func resolveIndexTierOption(opts IndexOptions) (IndexTier, error) {
	if strings.TrimSpace(string(opts.Tier)) != "" {
		return ParseIndexTier(string(opts.Tier))
	}
	cfg, _, err := LoadIndexConfigOptional()
	if err != nil {
		return "", err
	}
	return IndexTier(cfg.Tier), nil
}

func tierAllowsGoList(tier IndexTier) bool {
	return tier == IndexTierBalanced || tier == IndexTierFull
}
