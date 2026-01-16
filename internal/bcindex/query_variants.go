package bcindex

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	queryVariantTokenLimit  = 16
	queryVariantMinTokenLen = 2
)

type queryVariant struct {
	Text   string
	Weight float64
	Source string
}

type weightedHit struct {
	Hit    SearchHit
	Weight float64
}

func buildQueryVariants(query string) []queryVariant {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	variants := []queryVariant{{Text: q, Weight: 1.0, Source: "query"}}
	tokens := tokenizeQuery(q)
	if len(tokens) >= 2 {
		tokenQuery := strings.Join(tokens, " ")
		if tokenQuery != q {
			variants = append(variants, queryVariant{Text: tokenQuery, Weight: 0.85, Source: "tokens"})
		}
	}
	return variants
}

func tokenizeQuery(query string) []string {
	if query == "" {
		return nil
	}
	tokens := make([]string, 0, queryVariantTokenLimit)
	seen := make(map[string]struct{}, queryVariantTokenLimit)
	addToken := func(token string) bool {
		if len(tokens) >= queryVariantTokenLimit {
			return false
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return true
		}
		if isASCIIWord(token) {
			token = strings.ToLower(token)
		}
		if len([]rune(token)) < queryVariantMinTokenLen {
			return true
		}
		if _, ok := seen[token]; ok {
			return true
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
		return true
	}

	for _, word := range asciiWordPattern.FindAllString(query, -1) {
		for _, part := range splitASCIIWord(word) {
			if !addToken(part) {
				return tokens
			}
		}
	}

	var cjkBuf []rune
	for _, r := range []rune(query) {
		if unicode.Is(unicode.Han, r) {
			cjkBuf = append(cjkBuf, r)
			continue
		}
		if len(cjkBuf) > 0 {
			addCJKTokens(cjkBuf, addToken)
			cjkBuf = cjkBuf[:0]
			if len(tokens) >= queryVariantTokenLimit {
				return tokens
			}
		}
	}
	if len(cjkBuf) > 0 {
		addCJKTokens(cjkBuf, addToken)
	}
	return tokens
}

func splitASCIIWord(word string) []string {
	word = strings.Trim(word, "_")
	if word == "" {
		return nil
	}
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '_' })
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		camelParts := splitCamelCase(part)
		if len(camelParts) == 0 {
			tokens = append(tokens, part)
			continue
		}
		if len(camelParts) == 1 {
			tokens = append(tokens, camelParts[0])
			continue
		}
		tokens = append(tokens, camelParts...)
	}
	if len(tokens) == 0 {
		return []string{word}
	}
	return tokens
}

func splitCamelCase(word string) []string {
	if word == "" {
		return nil
	}
	isLower := func(ch byte) bool { return ch >= 'a' && ch <= 'z' }
	isUpper := func(ch byte) bool { return ch >= 'A' && ch <= 'Z' }
	isDigit := func(ch byte) bool { return ch >= '0' && ch <= '9' }
	isAlpha := func(ch byte) bool { return isLower(ch) || isUpper(ch) }

	var parts []string
	start := 0
	for i := 1; i < len(word); i++ {
		prev := word[i-1]
		curr := word[i]
		if isUpper(prev) && isLower(curr) && i-1 > start {
			parts = append(parts, word[start:i-1])
			start = i - 1
			continue
		}
		if isLower(prev) && isUpper(curr) {
			parts = append(parts, word[start:i])
			start = i
			continue
		}
		if isAlpha(prev) && isDigit(curr) {
			parts = append(parts, word[start:i])
			start = i
			continue
		}
		if isDigit(prev) && isAlpha(curr) {
			parts = append(parts, word[start:i])
			start = i
		}
	}
	if start < len(word) {
		parts = append(parts, word[start:])
	}
	return parts
}

func addCJKTokens(segment []rune, add func(string) bool) {
	if len(segment) == 0 {
		return
	}
	if len(segment) <= 2 {
		add(string(segment))
		return
	}
	if len(segment) <= 4 {
		if !add(string(segment)) {
			return
		}
	}
	for i := 0; i+1 < len(segment); i++ {
		if !add(string(segment[i : i+2])) {
			return
		}
	}
	if len(segment) >= 3 {
		for i := 0; i+2 < len(segment); i++ {
			if !add(string(segment[i : i+3])) {
				return
			}
		}
	}
}

func expandCandidateLimit(limit int, variants int) int {
	if limit <= 0 {
		if variants > 1 {
			return 15
		}
		return 10
	}
	if variants <= 1 {
		return limit
	}
	if limit >= 100 {
		return limit
	}
	if limit < 20 {
		return limit * 2
	}
	return limit + 10
}

func mergeWeightedHits(weighted []weightedHit, limit int) []SearchHit {
	if len(weighted) == 0 {
		return nil
	}
	type aggHit struct {
		hit       SearchHit
		score     float64
		matches   int
		bestScore float64
	}
	hitMap := make(map[string]aggHit)
	for _, entry := range weighted {
		hit := entry.Hit
		if hit.File == "" {
			continue
		}
		key := variantHitKey(hit)
		weightedScore := hit.Score * entry.Weight
		if existing, ok := hitMap[key]; ok {
			existing.score += weightedScore
			existing.matches++
			if weightedScore > existing.bestScore {
				existing.bestScore = weightedScore
				existing.hit = hit
			}
			if existing.hit.Snippet == "" && hit.Snippet != "" {
				existing.hit.Snippet = hit.Snippet
			}
			if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
				existing.hit.LineEnd = hit.LineEnd
			}
			hitMap[key] = existing
			continue
		}
		hitMap[key] = aggHit{hit: hit, score: weightedScore, matches: 1, bestScore: weightedScore}
	}
	results := make([]SearchHit, 0, len(hitMap))
	for _, entry := range hitMap {
		bonus := 0.0
		if entry.matches > 1 {
			bonus = 0.05 * float64(entry.matches-1)
		}
		entry.hit.Score = entry.score + bonus
		results = append(results, entry.hit)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if results[i].File == results[j].File {
				return results[i].Line < results[j].Line
			}
			return results[i].File < results[j].File
		}
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func variantHitKey(hit SearchHit) string {
	parts := []string{hit.Kind, hit.File, strconv.Itoa(hit.Line)}
	if hit.LineEnd > 0 {
		parts = append(parts, strconv.Itoa(hit.LineEnd))
	}
	if hit.Name != "" {
		parts = append(parts, hit.Name)
	}
	return strings.Join(parts, ":")
}
