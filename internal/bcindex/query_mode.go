package bcindex

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type QueryMode string

const (
	QueryModeAuto         QueryMode = "auto"
	QueryModeSearch       QueryMode = "search"
	QueryModeContext      QueryMode = "context"
	QueryModeImpact       QueryMode = "impact"
	QueryModeArchitecture QueryMode = "architecture"
	QueryModeQuality      QueryMode = "quality"
)

type QueryReport struct {
	Mode         string             `json:"mode"`
	SelectedMode string             `json:"selected_mode,omitempty"`
	Query        string             `json:"query"`
	Hits         []SearchHit        `json:"hits,omitempty"`
	Stats        map[string]int     `json:"stats,omitempty"`
	TopDepends   []RelationPairStat `json:"top_depends_on,omitempty"`
	Truncated    bool               `json:"truncated,omitempty"`
	Truncation   string             `json:"truncation,omitempty"`
}

func ParseQueryMode(value string) (QueryMode, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return QueryModeAuto, nil
	}
	switch v {
	case string(QueryModeAuto),
		string(QueryModeSearch),
		string(QueryModeContext),
		string(QueryModeImpact),
		string(QueryModeArchitecture),
		string(QueryModeQuality):
		return QueryMode(v), nil
	default:
		return "", fmt.Errorf("unknown query mode: %s", value)
	}
}

func QueryRepoMode(paths RepoPaths, meta *RepoMeta, query string, mode QueryMode, topK int) (QueryReport, error) {
	report := QueryReport{Mode: string(mode), Query: query}
	resolvedMode := mode
	if mode == QueryModeAuto {
		resolvedMode = detectQueryMode(query)
		report.SelectedMode = string(resolvedMode)
	}
	switch resolvedMode {
	case QueryModeContext, QueryModeImpact:
		var hits []SearchHit
		var err error
		if resolvedMode == QueryModeContext {
			hits, err = queryContext(paths, meta, query, topK)
		} else {
			hits, err = QueryRepo(paths, meta, query, "mixed", topK)
		}
		if err != nil {
			return report, err
		}
		report.Hits = hits
		return report, nil
	case QueryModeArchitecture:
		stats, topPairs, err := buildArchitectureStats(paths)
		if err != nil {
			return report, err
		}
		report.Stats = stats
		report.TopDepends = topPairs
		return report, nil
	case QueryModeQuality:
		stats, err := buildQualityStats(paths)
		if err != nil {
			return report, err
		}
		report.Stats = stats
		return report, nil
	case QueryModeSearch:
		hits, err := querySearch(paths, meta, query, topK)
		if err != nil {
			return report, err
		}
		report.Hits = hits
		return report, nil
	default:
		return report, fmt.Errorf("unsupported query mode: %s", mode)
	}
}

func FormatQueryReport(report QueryReport, maxChars int) (string, bool, string) {
	if maxChars <= 0 {
		maxChars = defaultQueryConfig().MaxContextChars
	}
	displayMode := report.Mode
	resolvedMode := QueryMode(report.Mode)
	if resolvedMode == QueryModeAuto && report.SelectedMode != "" {
		resolvedMode = QueryMode(report.SelectedMode)
		displayMode = fmt.Sprintf("%s (%s)", report.Mode, report.SelectedMode)
	}
	var b strings.Builder
	switch resolvedMode {
	case QueryModeContext:
		b.WriteString(fmt.Sprintf("mode: %s\nquery: %s\n", displayMode, report.Query))
		writeHits(&b, report.Hits)
	case QueryModeImpact:
		b.WriteString(fmt.Sprintf("mode: %s\nquery: %s\n", displayMode, report.Query))
		writeImpact(&b, report.Hits)
	case QueryModeArchitecture:
		b.WriteString(fmt.Sprintf("mode: %s\n", displayMode))
		writeStats(&b, report.Stats)
		writeTopDepends(&b, report.TopDepends)
	case QueryModeQuality:
		b.WriteString(fmt.Sprintf("mode: %s\n", displayMode))
		writeStats(&b, report.Stats)
	case QueryModeSearch:
		b.WriteString(fmt.Sprintf("mode: %s\nquery: %s\n", displayMode, report.Query))
		writeHits(&b, report.Hits)
	default:
		b.WriteString(fmt.Sprintf("mode: %s\n", displayMode))
		writeHits(&b, report.Hits)
	}
	out := b.String()
	truncated := false
	truncation := ""
	if countRunes(out) > maxChars {
		out = truncateWithNotice(out, maxChars)
		truncated = true
		truncation = fmt.Sprintf("truncated to max_context_chars=%d", maxChars)
	}
	return out, truncated, truncation
}

func writeHits(b *strings.Builder, hits []SearchHit) {
	if len(hits) == 0 {
		b.WriteString("no results\n")
		return
	}
	for i, hit := range hits {
		name := hit.Name
		if name == "" {
			name = "-"
		}
		line := hit.Line
		if line <= 0 {
			line = 1
		}
		b.WriteString(fmt.Sprintf("%d) %s\t%s\t%s:%d\n", i+1, hit.Kind, name, hit.File, line))
		if hit.Snippet != "" {
			b.WriteString(fmt.Sprintf("   snippet: %s\n", strings.TrimSpace(hit.Snippet)))
		}
		if summary := formatRelationSummary(hit.Relations); summary != "" {
			b.WriteString(fmt.Sprintf("   relations: %s\n", summary))
		}
		if docSummary := formatDocLinkSummary(hit.DocLinks); docSummary != "" {
			b.WriteString(fmt.Sprintf("   doc_links: %s\n", docSummary))
		}
	}
}

func writeImpact(b *strings.Builder, hits []SearchHit) {
	if len(hits) == 0 {
		b.WriteString("no results\n")
		return
	}
	for i, hit := range hits {
		line := hit.Line
		if line <= 0 {
			line = 1
		}
		b.WriteString(fmt.Sprintf("%d) %s:%d\n", i+1, hit.File, line))
		if summary := formatRelationSummary(hit.Relations); summary != "" {
			b.WriteString(fmt.Sprintf("   relations: %s\n", summary))
		}
	}
}

func writeStats(b *strings.Builder, stats map[string]int) {
	if len(stats) == 0 {
		b.WriteString("no stats\n")
		return
	}
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("%s: %d\n", key, stats[key]))
	}
}

func writeTopDepends(b *strings.Builder, pairs []RelationPairStat) {
	if len(pairs) == 0 {
		return
	}
	b.WriteString("top_depends_on:\n")
	for _, pair := range pairs {
		b.WriteString(fmt.Sprintf("- %s -> %s (%d)\n", pair.FromRef, pair.ToRef, pair.Count))
	}
}

func formatRelationSummary(relations []RelationSummary) string {
	if len(relations) == 0 {
		return ""
	}
	parts := make([]string, 0, len(relations))
	for _, summary := range relations {
		if len(summary.Edges) == 0 {
			continue
		}
		targets := make([]string, 0, len(summary.Edges))
		for _, edge := range summary.Edges {
			if edge.ToRef == "" {
				continue
			}
			targets = append(targets, edge.ToRef)
		}
		if len(targets) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", summary.Kind, strings.Join(targets, ", ")))
	}
	return strings.Join(parts, "; ")
}

func formatDocLinkSummary(links []DocLinkHit) string {
	if len(links) == 0 {
		return ""
	}
	symbols := make([]string, 0, len(links))
	seen := make(map[string]struct{})
	for _, link := range links {
		if link.Symbol == "" {
			continue
		}
		if _, ok := seen[link.Symbol]; ok {
			continue
		}
		seen[link.Symbol] = struct{}{}
		symbols = append(symbols, link.Symbol)
	}
	if len(symbols) == 0 {
		return ""
	}
	return strings.Join(symbols, ", ")
}

func detectQueryMode(query string) QueryMode {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return QueryModeContext
	}
	tokens := asciiWordSet(q)
	if containsAny(q, tokens, queryModeQualityKeywords()) {
		return QueryModeQuality
	}
	if containsAny(q, tokens, queryModeArchitectureKeywords()) {
		return QueryModeArchitecture
	}
	if isQuestionQuery(query) {
		if containsAny(q, tokens, queryModeImpactKeywords()) {
			return QueryModeImpact
		}
		if containsAny(q, tokens, queryModeSearchKeywords()) {
			return QueryModeSearch
		}
		return QueryModeContext
	}
	if containsAny(q, tokens, queryModeImpactKeywords()) {
		return QueryModeImpact
	}
	if containsAny(q, tokens, queryModeSearchKeywords()) {
		return QueryModeSearch
	}
	return QueryModeContext
}

func containsAny(query string, tokens map[string]struct{}, keywords []string) bool {
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if isASCIIWord(kw) {
			if _, ok := tokens[kw]; ok {
				return true
			}
			continue
		}
		if strings.Contains(query, kw) {
			return true
		}
	}
	return false
}

var asciiWordPattern = regexp.MustCompile(`[A-Za-z0-9_]+`)

func asciiWordSet(query string) map[string]struct{} {
	words := asciiWordPattern.FindAllString(query, -1)
	if len(words) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		set[strings.ToLower(word)] = struct{}{}
	}
	return set
}

func isASCIIWord(word string) bool {
	for i := 0; i < len(word); i++ {
		ch := word[i]
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' {
			continue
		}
		return false
	}
	return true
}

func queryModeQualityKeywords() []string {
	return []string{
		"质量", "覆盖", "覆盖率", "索引率", "统计", "指标",
		"quality", "coverage", "stats", "metric",
	}
}

func queryModeArchitectureKeywords() []string {
	return []string{
		"架构", "结构", "模块", "拓扑", "依赖图", "关系图",
		"architecture", "structure", "module", "dependency graph", "graph",
	}
}

func queryModeImpactKeywords() []string {
	return []string{
		"影响", "改动", "修改", "变更", "影响范围", "影响面", "依赖", "引用", "调用",
		"impact", "affect", "depend", "dependency", "caller", "callee",
	}
}

func queryModeSearchKeywords() []string {
	return []string{
		"在哪", "哪里", "哪个文件", "哪个包", "文件", "路径", "位置", "定位", "目录",
		"package", "file", "path", "locate", "located", "where",
	}
}

func buildArchitectureStats(paths RepoPaths) (map[string]int, []RelationPairStat, error) {
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return nil, nil, err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return nil, nil, err
	}

	importsCount, err := store.CountRelationsByKind(RelationKindImports)
	if err != nil {
		return nil, nil, err
	}
	dependsCount, err := store.CountRelationsByKind(RelationKindDependsOn)
	if err != nil {
		return nil, nil, err
	}
	topDepends, err := store.ListTopRelationPairs(RelationKindDependsOn, 10)
	if err != nil {
		return nil, nil, err
	}
	stats := map[string]int{
		"imports":    importsCount,
		"depends_on": dependsCount,
	}
	return stats, topDepends, nil
}

func buildQualityStats(paths RepoPaths) (map[string]int, error) {
	store, err := OpenSymbolStore(symbolDBPath(paths))
	if err != nil {
		return nil, err
	}
	defer store.Close()

	if err := store.InitSchema(false); err != nil {
		return nil, err
	}

	symbols, err := store.CountSymbols()
	if err != nil {
		return nil, err
	}
	relations, err := store.CountRelations()
	if err != nil {
		return nil, err
	}
	docLinks, err := store.CountDocLinks()
	if err != nil {
		return nil, err
	}

	textIndex, err := OpenTextIndex(paths.TextDir)
	if err != nil {
		return nil, err
	}
	defer textIndex.Close()

	docCount, err := textIndex.DocCount()
	if err != nil {
		return nil, err
	}

	stats := map[string]int{
		"symbols":   symbols,
		"relations": relations,
		"doc_links": docLinks,
		"text_docs": int(docCount),
	}
	return stats, nil
}

func countRunes(text string) int {
	return len([]rune(text))
}

func truncateWithNotice(text string, maxChars int) string {
	if maxChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	notice := fmt.Sprintf("\n...[truncated max_context_chars=%d]\n", maxChars)
	limit := maxChars - len([]rune(notice))
	if limit < 0 {
		limit = 0
	}
	return string(runes[:limit]) + notice
}
