package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// SetupLogging 根据子命令与仓库根目录初始化日志配置。
// 返回设置失败时的 error。
func SetupLogging(subcommand string, repoRoot string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(homeDir, ".bcindex", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	repoName := sanitizeRepoName(filepath.Base(repoRoot))
	hash := sha1.Sum([]byte(repoRoot))
	suffix := hex.EncodeToString(hash[:])[:8]
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("bcindex-%s-%s-%s-%s.log", subcommand, repoName, timestamp, suffix)
	logPath := filepath.Join(logDir, filename)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	log.Printf("Log file: %s", logPath)
	return nil
}
