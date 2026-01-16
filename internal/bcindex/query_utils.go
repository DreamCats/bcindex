package bcindex

import "strings"

func joinSources(hasSymbol, hasText, hasVector bool) string {
	parts := make([]string, 0, 3)
	if hasSymbol {
		parts = append(parts, "symbol")
	}
	if hasText {
		parts = append(parts, "text")
	}
	if hasVector {
		parts = append(parts, "vector")
	}
	return strings.Join(parts, "+")
}
