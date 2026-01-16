package bcindex

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type IndexLogger struct {
	mu       sync.Mutex
	file     *os.File
	repoRoot string
	repoID   string
}

type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Details   map[string]interface{}
}

var globalLogger *IndexLogger
var loggerMu sync.Mutex

func InitIndexLogger(repoRoot string) (*IndexLogger, error) {
	repoID := storeRepoID(repoRoot)
	baseDirPath, err := baseDir()
	if err != nil {
		return nil, fmt.Errorf("get base dir: %w", err)
	}
	logDir := filepath.Join(baseDirPath, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	logFile := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", repoID[:8], timestamp))

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	logger := &IndexLogger{
		file:     file,
		repoRoot: repoRoot,
		repoID:   repoID,
	}

	loggerMu.Lock()
	globalLogger = logger
	loggerMu.Unlock()

	logger.log("INFO", "IndexLogger initialized", map[string]interface{}{
		"repo_root": repoRoot,
		"repo_id":   repoID,
		"log_file":  logFile,
	})

	return logger, nil
}

func (l *IndexLogger) log(level string, message string, details map[string]interface{}) {
	if l == nil {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Details:   details,
	}

	logLine := fmt.Sprintf("[%s] %s: %s", entry.Timestamp.Format("2006-01-02 15:04:05.000"), entry.Level, entry.Message)
	for k, v := range entry.Details {
		logLine += fmt.Sprintf(" %s=%v", k, v)
	}
	logLine += "\n"

	l.mu.Lock()
	defer l.mu.Unlock()
	l.file.WriteString(logLine)
}

func (l *IndexLogger) Info(message string, details map[string]interface{}) {
	l.log("INFO", message, details)
}

func (l *IndexLogger) Warn(message string, details map[string]interface{}) {
	l.log("WARN", message, details)
}

func (l *IndexLogger) Error(message string, details map[string]interface{}) {
	l.log("ERROR", message, details)
}

func (l *IndexLogger) Debug(message string, details map[string]interface{}) {
	l.log("DEBUG", message, details)
}

func (l *IndexLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.Info("IndexLogger closing", nil)
	return l.file.Close()
}

func GetGlobalLogger() *IndexLogger {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	return globalLogger
}

func LogInfo(message string, details map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Info(message, details)
	}
}

func LogWarn(message string, details map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Warn(message, details)
	}
}

func LogError(message string, details map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Error(message, details)
	}
}

func LogDebug(message string, details map[string]interface{}) {
	if logger := GetGlobalLogger(); logger != nil {
		logger.Debug(message, details)
	}
}
