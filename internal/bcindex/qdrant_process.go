package bcindex

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type QdrantProcess struct {
	cmd     *exec.Cmd
	pidFile string
	started bool
}

func EnsureQdrantRunning(ctx context.Context, cfg VectorConfig) (*QdrantProcess, error) {
	if cfg.QdrantPath == "" || !cfg.QdrantAutoStart {
		return nil, nil
	}
	if qdrantHealthy(cfg.QdrantURL) {
		return nil, nil
	}

	proc := &QdrantProcess{pidFile: qdrantPIDPath()}
	if err := proc.start(ctx, cfg); err != nil {
		return nil, err
	}
	return proc, nil
}

func (p *QdrantProcess) Stop() {
	if p == nil || !p.started || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	time.Sleep(300 * time.Millisecond)
	_ = p.cmd.Process.Kill()
	_ = os.Remove(p.pidFile)
}

func (p *QdrantProcess) start(ctx context.Context, cfg VectorConfig) error {
	bin, err := resolveQdrantBinary(cfg)
	if err != nil {
		return err
	}

	logPath, err := qdrantLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"QDRANT__STORAGE__PATH="+cfg.QdrantPath,
		fmt.Sprintf("QDRANT__SERVICE__HTTP_PORT=%d", cfg.QdrantHTTPPort),
		fmt.Sprintf("QDRANT__SERVICE__GRPC_PORT=%d", cfg.QdrantGRPCPort),
		"QDRANT__SERVICE__HOST=127.0.0.1",
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.started = true
	_ = os.WriteFile(p.pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if qdrantHealthy(cfg.QdrantURL) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	p.Stop()
	return fmt.Errorf("qdrant did not become healthy in time")
}

func qdrantHealthy(baseURL string) bool {
	client := http.Client{Timeout: 2 * time.Second}
	healthURL := strings.TrimRight(baseURL, "/") + "/healthz"
	if resp, err := client.Get(healthURL); err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true
		}
	}
	collectionsURL := strings.TrimRight(baseURL, "/") + "/collections"
	if resp, err := client.Get(collectionsURL); err == nil {
		_ = resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 300
	}
	return false
}

func qdrantPIDPath() string {
	base, err := baseDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "bcindex-qdrant.pid")
	}
	return filepath.Join(base, "qdrant.pid")
}

func qdrantLogPath() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "logs", "qdrant.log"), nil
}
