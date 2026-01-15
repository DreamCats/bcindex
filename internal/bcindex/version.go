package bcindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ReadVersion(root string) (string, error) {
	path := filepath.Join(root, "PROJECT_META.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	version := parseVersion(string(data))
	if version == "" {
		return "", fmt.Errorf("version not found in %s", path)
	}
	return version, nil
}

func parseVersion(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "版本号：") {
			parts := strings.SplitN(line, "版本号：", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "version:") {
			return strings.TrimSpace(line[len("version:"):])
		}
	}
	return ""
}
