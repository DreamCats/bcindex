package bcindex

import (
	"fmt"
	"os/exec"
	"strings"
)

type FileChange struct {
	Status  string
	Path    string
	OldPath string
}

func gitDiffChanges(root, rev string) ([]FileChange, error) {
	if strings.TrimSpace(rev) == "" {
		return nil, fmt.Errorf("diff revision is required")
	}
	cmd := exec.Command("git", "-C", root, "diff", "--name-status", rev)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	return parseNameStatusOutput(string(out)), nil
}

func gitStatusChanges(root string) ([]FileChange, error) {
	cmd := exec.Command("git", "-C", root, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	return parseStatusOutput(string(out)), nil
}

func parseNameStatusOutput(output string) []FileChange {
	var changes []FileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		if strings.HasPrefix(status, "R") && len(fields) >= 3 {
			changes = append(changes, FileChange{
				Status:  "R",
				OldPath: fields[1],
				Path:    fields[2],
			})
			continue
		}
		changes = append(changes, FileChange{
			Status: status[:1],
			Path:   fields[1],
		})
	}
	return changes
}

func parseStatusOutput(output string) []FileChange {
	var changes []FileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}
		status := line[:2]
		pathPart := strings.TrimSpace(line[2:])
		if strings.HasPrefix(status, "R") && strings.Contains(pathPart, "->") {
			parts := strings.Split(pathPart, "->")
			if len(parts) == 2 {
				changes = append(changes, FileChange{
					Status:  "R",
					OldPath: strings.TrimSpace(parts[0]),
					Path:    strings.TrimSpace(parts[1]),
				})
			}
			continue
		}
		if strings.HasPrefix(status, "??") {
			changes = append(changes, FileChange{Status: "A", Path: pathPart})
			continue
		}
		if strings.Contains(status, "D") {
			changes = append(changes, FileChange{Status: "D", Path: pathPart})
			continue
		}
		if strings.Contains(status, "A") {
			changes = append(changes, FileChange{Status: "A", Path: pathPart})
			continue
		}
		changes = append(changes, FileChange{Status: "M", Path: pathPart})
	}
	return changes
}
