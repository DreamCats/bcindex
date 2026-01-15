package bcindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureIndex(paths RepoPaths, qtype string) error {
	q := strings.ToLower(strings.TrimSpace(qtype))
	if q == "" {
		q = "mixed"
	}
	needText := q == "text" || q == "mixed"
	needSymbol := q == "symbol" || q == "mixed"

	if needText {
		metaPath := filepath.Join(paths.TextDir, "index_meta.json")
		if _, err := os.Stat(metaPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("text index not found. run: bcindex index --root %s --full", paths.Root)
			}
			return fmt.Errorf("text index check failed: %w", err)
		}
	}
	if needSymbol {
		if _, err := os.Stat(symbolDBPath(paths)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("symbol index not found. run: bcindex index --root %s --full", paths.Root)
			}
			return fmt.Errorf("symbol index check failed: %w", err)
		}
	}
	return nil
}
