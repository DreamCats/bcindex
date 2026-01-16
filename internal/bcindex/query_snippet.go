package bcindex

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func readLine(root, rel string, target int) string {
	if target <= 0 {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 1
	for scanner.Scan() {
		if line == target {
			return strings.TrimSpace(scanner.Text())
		}
		line++
	}
	return ""
}

func readLinesRange(root, rel string, start, end, maxLines int) string {
	if start <= 0 {
		return ""
	}
	if end <= 0 {
		end = start
	}
	if end < start {
		end = start
	}
	if maxLines > 0 && end-start+1 > maxLines {
		end = start + maxLines - 1
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 1
	var out []string
	for scanner.Scan() {
		if line >= start && line <= end {
			out = append(out, scanner.Text())
		}
		if line > end {
			break
		}
		line++
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func findMatchLine(root, rel, query string) (int, string) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	line := 1
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, query) {
			return line, strings.TrimSpace(text)
		}
		line++
	}
	return 0, ""
}
