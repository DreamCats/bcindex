package bcindex

import (
	"sort"
	"strconv"
	"strings"
)

func queryContext(paths RepoPaths, meta *RepoMeta, query string, topK int) ([]SearchHit, error) {
	question := isQuestionQuery(query)
	identifier := extractQueryIdentifier(query)
	preferCode := identifier != "" && isImplementationQuery(query)
	docQuestion := question && !preferCode
	vectorCfg, vectorEnabled, err := loadVectorConfigForQuery()
	if err != nil {
		return nil, err
	}
	candidateTop := topK
	if vectorEnabled && vectorCfg.VectorRerankTop > candidateTop {
		candidateTop = vectorCfg.VectorRerankTop
	}

	searchQuery := query
	if preferCode {
		searchQuery = identifier
	}
	textQuery := searchQuery
	if !preferCode {
		textQuery = expandDocQuery(searchQuery)
	}
	var textHits []SearchHit
	if preferCode {
		textHits, err = queryTextContextExact(paths, meta, textQuery, candidateTop)
	} else {
		textHits, err = queryTextContext(paths, meta, textQuery, candidateTop)
	}
	if err != nil {
		return nil, err
	}
	var symbolHits []SearchHit
	if preferCode {
		symbolHits, err = querySymbolsExact(paths, searchQuery, candidateTop)
	} else {
		symbolHits, err = querySymbols(paths, searchQuery, candidateTop)
	}
	if err != nil {
		return nil, err
	}
	vectorHits, err := queryVectorCandidates(paths, meta, searchQuery, topK, vectorCfg, vectorEnabled, textHits, symbolHits)
	if err != nil {
		vectorHits = nil
	}
	if preferCode {
		if hasNonDocHit(textHits) || hasNonDocHit(symbolHits) || hasNonDocHit(vectorHits) {
			textHits = filterDocHits(textHits)
			vectorHits = filterDocHits(vectorHits)
		}
	}
	if preferCode && len(textHits) == 0 && len(symbolHits) == 0 && len(vectorHits) == 0 {
		preferCode = false
		docQuestion = question
		textQuery = expandDocQuery(query)
		textHits, err = queryTextContext(paths, meta, textQuery, candidateTop)
		if err != nil {
			return nil, err
		}
		symbolHits, err = querySymbols(paths, query, candidateTop)
		if err != nil {
			return nil, err
		}
		vectorHits, err = queryVectorCandidates(paths, meta, query, topK, vectorCfg, vectorEnabled, textHits, symbolHits)
		if err != nil {
			vectorHits = nil
		}
	}

	type rankedHit struct {
		hit      SearchHit
		text     *SearchHit
		vector   *SearchHit
		symbol   *SearchHit
		priority int
		docBoost float64
	}
	hitMap := make(map[string]rankedHit)
	for _, hit := range symbolHits {
		key := hitKey(hit)
		hit.Score = 1.0
		h := hit
		hitMap[key] = rankedHit{hit: h, symbol: &h, priority: 2}
	}
	for _, hit := range textHits {
		key := hitKey(hit)
		if existing, ok := hitMap[key]; ok {
			if existing.priority >= 2 {
				if existing.hit.Snippet == "" {
					existing.hit.Snippet = hit.Snippet
				}
				if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
					existing.hit.LineEnd = hit.LineEnd
				}
				h := hit
				existing.text = &h
				hitMap[key] = existing
				continue
			}
			if hit.Score > existing.hit.Score {
				h := hit
				existing.hit = h
				existing.text = &h
				existing.priority = 1
				if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
					existing.hit.LineEnd = hit.LineEnd
				}
				hitMap[key] = existing
			}
			continue
		}
		h := hit
		hitMap[key] = rankedHit{hit: h, text: &h, priority: 1}
	}
	for _, hit := range vectorHits {
		key := hitKey(hit)
		if existing, ok := hitMap[key]; ok {
			h := hit
			existing.vector = &h
			if existing.hit.Snippet == "" {
				existing.hit.Snippet = hit.Snippet
			}
			if existing.hit.LineEnd == 0 && hit.LineEnd > 0 {
				existing.hit.LineEnd = hit.LineEnd
			}
			hitMap[key] = existing
			continue
		}
		h := hit
		hitMap[key] = rankedHit{hit: h, vector: &h, priority: 0}
	}

	var merged []rankedHit
	for _, hit := range hitMap {
		vectorScore := 0.0
		textScore := 0.0
		symbolBoost := 0.0
		if hit.vector != nil {
			vectorScore = hit.vector.Score
		}
		if hit.text != nil {
			textScore = hit.text.Score
		}
		if hit.symbol != nil {
			symbolBoost = 0.5
		}
		docBoost := docBoostForHit(hit.hit, docQuestion)
		codeBoost := codeBoostForHit(hit.hit)
		if preferCode && isDocFile(hit.hit.File) {
			docBoost -= 0.8
			if docBoost < 0 {
				docBoost = 0
			}
		}
		hit.docBoost = docBoost
		vectorWeight := 0.5
		textWeight := 0.4
		symbolWeight := 0.1
		if question {
			vectorWeight = 0.55
			textWeight = 0.35
			symbolWeight = 0.1
		}
		if preferCode {
			vectorWeight = 0.35
			textWeight = 0.35
			symbolWeight = 0.3
		}
		hit.hit.Score = vectorWeight*vectorScore + textWeight*textScore + symbolWeight*symbolBoost + docBoost + codeBoost
		hit.hit.Source = joinSources(hit.symbol != nil, hit.text != nil, hit.vector != nil)
		merged = append(merged, hit)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].docBoost != merged[j].docBoost {
			return merged[i].docBoost > merged[j].docBoost
		}
		if merged[i].priority != merged[j].priority {
			return merged[i].priority > merged[j].priority
		}
		if merged[i].hit.Score == merged[j].hit.Score {
			if merged[i].hit.File == merged[j].hit.File {
				return merged[i].hit.Line < merged[j].hit.Line
			}
			return merged[i].hit.File < merged[j].hit.File
		}
		return merged[i].hit.Score > merged[j].hit.Score
	})
	results := make([]SearchHit, 0, len(merged))
	for _, hit := range merged {
		results = append(results, hit.hit)
	}
	results = filterContextHits(results, topK, docQuestion)
	if preferCode {
		results = filterDocHitsIfCodeExists(results)
	}
	return enrichMixedHits(paths, results)
}

func hitKey(hit SearchHit) string {
	return hit.File + ":" + strconv.Itoa(hit.Line)
}

func docBoostForHit(hit SearchHit, question bool) float64 {
	boost := docBoostForFile(hit.File)
	if question && isDocFile(hit.File) {
		boost += 0.5
	}
	sectionBoost := docSectionBoost(hit.Snippet)
	if isReadmeFile(hit.File) {
		sectionBoost += readmeSectionBoost(hit.Snippet)
	}
	return boost + sectionBoost
}

func codeBoostForHit(hit SearchHit) float64 {
	if hit.File == "" {
		return 0.0
	}
	lower := strings.ToLower(hit.File)
	if strings.HasSuffix(lower, ".go") {
		return 0.3
	}
	return 0.0
}

func docBoostForFile(path string) float64 {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "readme.md") {
		return 1.5
	}
	if strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "reference/") {
		return 1.0
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return 0.8
	}
	return 0.0
}

func isReadmeFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), "readme.md")
}

func isDocFile(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "readme.md") {
		return true
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return true
	}
	return strings.HasPrefix(lower, "docs/") || strings.HasPrefix(lower, "reference/")
}

func isQuestionQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	if strings.ContainsAny(q, "?？") {
		return true
	}
	keywords := []string{
		"是什么", "做什么", "干什么", "用途", "作用", "目的", "意义",
		"如何", "为啥", "为什么", "原因", "介绍", "概述", "概览", "说明",
		"what", "why", "purpose", "overview", "about", "introduce",
	}
	for _, kw := range keywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func filterContextHits(hits []SearchHit, topK int, question bool) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	maxPerFile := 2
	seen := make(map[string]int)

	if question {
		var docs []SearchHit
		var others []SearchHit
		for _, hit := range hits {
			if hit.File == "" {
				continue
			}
			fileLimit := 1
			if isReadmeFile(hit.File) {
				fileLimit = 2
			}
			if seen[hit.File] >= fileLimit {
				continue
			}
			seen[hit.File]++
			if isDocFile(hit.File) {
				docs = append(docs, hit)
			} else {
				others = append(others, hit)
			}
		}
		out := append(docs, others...)
		if topK > 0 && len(out) > topK {
			out = out[:topK]
		}
		return out
	}

	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.File == "" {
			continue
		}
		if seen[hit.File] >= maxPerFile {
			continue
		}
		seen[hit.File]++
		out = append(out, hit)
		if topK > 0 && len(out) >= topK {
			break
		}
	}
	return out
}

func hasNonDocHit(hits []SearchHit) bool {
	for _, hit := range hits {
		if hit.File == "" {
			continue
		}
		if !isDocFile(hit.File) {
			return true
		}
	}
	return false
}

func filterDocHits(hits []SearchHit) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		if hit.File == "" {
			continue
		}
		if isDocFile(hit.File) {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func filterDocHitsIfCodeExists(hits []SearchHit) []SearchHit {
	if !hasNonDocHit(hits) {
		return hits
	}
	return filterDocHits(hits)
}

func docSectionBoost(snippet string) float64 {
	return headingKeywordBoost(snippet, docSectionKeywords())
}

func readmeSectionBoost(snippet string) float64 {
	boost := headingKeywordBoost(snippet, readmeSectionKeywords())
	if boost > 0 {
		return boost
	}
	if strings.Contains(snippet, "本项目") || strings.Contains(strings.ToLower(snippet), "this project") {
		return 0.3
	}
	return 0.0
}

func headingKeywordBoost(snippet string, keywords []string) float64 {
	if snippet == "" {
		return 0
	}
	lines := strings.Split(snippet, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(trimmed)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return 0.9
			}
		}
	}
	return 0
}

func readmeSectionKeywords() []string {
	return []string{
		"功能", "特性", "目标", "简介", "介绍", "背景", "概述", "用途", "范围",
		"getting started", "overview", "introduction", "what", "features", "purpose",
	}
}

func docSectionKeywords() []string {
	return []string{
		"说明", "目标", "背景", "概述", "用途", "范围", "设计", "流程", "步骤", "流程图", "数据流",
		"overview", "introduction", "background", "purpose", "pipeline", "process", "flow",
	}
}

func isImplementationQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	implementationKeywords := []string{
		"实现", "实现逻辑", "逻辑", "原理", "机制", "源码", "细节",
		"怎么做", "怎么实现", "如何实现", "如何做", "怎么做的", "怎么工作",
		"implement", "implementation", "logic", "works", "how does",
	}
	for _, kw := range implementationKeywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func isConceptualQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	if strings.ContainsAny(q, "?？") {
		conceptKeywords := []string{
			"是什么", "做什么", "干什么", "用途", "作用", "目的", "意义",
			"为啥", "为什么", "原因", "介绍", "概述", "概览", "说明",
			"what", "why", "purpose", "overview", "about", "introduce", "explanation",
		}
		for _, kw := range conceptKeywords {
			if strings.Contains(q, kw) {
				return true
			}
		}
	}
	conceptKeywords := []string{
		"是什么", "做什么", "干什么", "用途", "作用", "目的", "意义",
		"为啥", "为什么", "原因", "介绍", "概述", "概览", "说明",
		"what", "why", "purpose", "overview", "about", "introduce", "explanation",
	}
	for _, kw := range conceptKeywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	return false
}

func expandDocQuery(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}
	lower := strings.ToLower(q)
	additions := make([]string, 0, 4)
	if strings.Contains(lower, "index") && !strings.Contains(q, "索引") {
		additions = append(additions, "索引")
	}
	if strings.Contains(lower, "query") && !strings.Contains(q, "查询") {
		additions = append(additions, "查询")
	}
	if strings.Contains(lower, "search") && !strings.Contains(q, "搜索") {
		additions = append(additions, "搜索")
	}
	if strings.Contains(lower, "watch") && !strings.Contains(q, "监听") {
		additions = append(additions, "监听")
	}
	if strings.Contains(lower, "diff") && !strings.Contains(q, "增量") {
		additions = append(additions, "增量")
	}
	if strings.Contains(lower, "process") && !strings.Contains(q, "流程") {
		additions = append(additions, "流程")
	}
	if strings.Contains(lower, "pipeline") && !strings.Contains(q, "流水线") {
		additions = append(additions, "流水线")
	}
	if strings.Contains(lower, "flow") && !strings.Contains(q, "流程") {
		additions = append(additions, "流程")
	}
	if len(additions) == 0 {
		return q
	}
	return q + " " + strings.Join(additions, " ")
}

func extractQueryIdentifier(query string) string {
	tokens := asciiWordPattern.FindAllString(query, -1)
	if len(tokens) == 0 {
		return ""
	}
	best := ""
	for _, token := range tokens {
		if len(token) < 3 {
			continue
		}
		if !hasASCIIAlpha(token) {
			continue
		}
		if len(token) > len(best) {
			best = token
		}
	}
	return best
}

func hasASCIIAlpha(token string) bool {
	for i := 0; i < len(token); i++ {
		ch := token[i]
		if ch >= 'A' && ch <= 'Z' {
			return true
		}
		if ch >= 'a' && ch <= 'z' {
			return true
		}
	}
	return false
}
