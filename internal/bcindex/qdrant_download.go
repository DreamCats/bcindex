package bcindex

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const defaultQdrantVersion = "v1.10.1"

func resolveQdrantBinary(cfg VectorConfig) (string, error) {
	if strings.TrimSpace(cfg.QdrantBin) != "" {
		return expandUserPath(cfg.QdrantBin), nil
	}
	if path, err := exec.LookPath(qdrantBinaryName()); err == nil {
		return path, nil
	}
	if cfg.QdrantPath == "" {
		return "", fmt.Errorf("qdrant binary not found; install qdrant or set qdrant_bin")
	}
	candidate := filepath.Join(cfg.QdrantPath, "bin", qdrantBinaryName())
	if fileExists(candidate) {
		return candidate, nil
	}
	candidate = filepath.Join(cfg.QdrantPath, qdrantBinaryName())
	if fileExists(candidate) {
		return candidate, nil
	}
	return downloadQdrantBinary(cfg.QdrantPath)
}

func downloadQdrantBinary(basePath string) (string, error) {
	version := strings.TrimSpace(os.Getenv("BCINDEX_QDRANT_VERSION"))
	if version == "" {
		version = defaultQdrantVersion
	}
	asset, archiveKind, err := qdrantAssetName()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://github.com/qdrant/qdrant/releases/download/%s/%s", version, asset)
	tmpFile, err := os.CreateTemp("", "bcindex-qdrant-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download qdrant: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download qdrant: status %d", resp.StatusCode)
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", fmt.Errorf("download qdrant: %w", err)
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	binDir := filepath.Join(basePath, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(binDir, qdrantBinaryName())

	switch archiveKind {
	case "tar.gz":
		if err := extractTarGz(tmpFile, outPath); err != nil {
			return "", err
		}
	case "zip":
		if err := extractZip(tmpFile, outPath); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported qdrant archive %q", archiveKind)
	}
	if err := os.Chmod(outPath, 0o755); err != nil {
		return "", err
	}
	return outPath, nil
}

func extractTarGz(file *os.File, outPath string) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != qdrantBinaryName() {
			continue
		}
		return writeExtractedFile(outPath, tr)
	}
	return fmt.Errorf("qdrant binary not found in archive")
}

func extractZip(file *os.File, outPath string) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}
	for _, f := range reader.File {
		if filepath.Base(f.Name) != qdrantBinaryName() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeExtractedFile(outPath, rc)
	}
	return fmt.Errorf("qdrant binary not found in archive")
}

func writeExtractedFile(outPath string, src io.Reader) error {
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer outFile.Close()
	_, err = io.Copy(outFile, src)
	return err
}

func qdrantBinaryName() string {
	if runtime.GOOS == "windows" {
		return "qdrant.exe"
	}
	return "qdrant"
}

func qdrantAssetName() (string, string, error) {
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "qdrant-x86_64-apple-darwin.tar.gz", "tar.gz", nil
		case "arm64":
			return "qdrant-aarch64-apple-darwin.tar.gz", "tar.gz", nil
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "qdrant-x86_64-unknown-linux-gnu.tar.gz", "tar.gz", nil
		case "arm64":
			return "qdrant-aarch64-unknown-linux-gnu.tar.gz", "tar.gz", nil
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "qdrant-x86_64-pc-windows-msvc.zip", "zip", nil
		}
	}
	return "", "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
