package internal

import (
	"fmt"
	"os"

	"github.com/DreamCats/bcindex/internal/config"
)

// LoadConfig 从指定路径读取并解析 YAML 配置文件。
// 返回填充后的 *config.Config 或解析错误。
func LoadConfig(configPath string) (*config.Config, error) {
	if configPath != "" {
		return config.LoadFromFile(configPath)
	}
	return config.Load()
}

// PrintConfigExample 向 stdout 打印一份完整的 YAML 配置示例。
// 供用户快速创建自定义配置文件。
func PrintConfigExample() {
	homeDir, _ := os.UserHomeDir()
	configPath := fmt.Sprintf("%s/.bcindex/config/bcindex.yaml", homeDir)

	fmt.Fprintf(os.Stderr, `Create a configuration file at %s:

# Embedding service configuration (required)
embedding:
  # Provider: "volcengine" | "openai"
  provider: volcengine

  # VolcEngine configuration
  api_key: your-volcengine-api-key
  endpoint: https://ark.cn-beijing.volces.com/api/v3
  model: doubao-embedding-vision-250615

  # Embedding parameters
  dimensions: 2048              # 1024 or 2048
  batch_size: 10                # Batch size for embedding requests
  encoding_format: float        # "float" or "base64"

# Database configuration
# Database is stored per-repository under ~/.bcindex/data/

# For OpenAI provider, use:
# embedding:
#   provider: openai
#   openai_api_key: your-openai-api-key
#   openai_model: text-embedding-3-small
#   dimensions: 1536

Usage:
  1. Create the config file
  2. Navigate to your Go project: cd /path/to/project
  3. Run: bcindex index
  4. Search: bcindex search "your query"
`, configPath)
}
