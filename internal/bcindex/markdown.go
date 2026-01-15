package bcindex

import "strings"

type MDChunk struct {
	Title     string
	Content   string
	LineStart int
	LineEnd   int
}

const markdownMaxChars = 1500

func ChunkMarkdown(src []byte) []MDChunk {
	lines := strings.Split(string(src), "\n")
	var chunks []MDChunk

	var current *MDChunk
	var currentLines []string
	var headingStack []string
	var headingLevels []int

	for i, line := range lines {
		level, title, ok := parseHeading(line)
		if ok {
			if current != nil {
				current.LineEnd = i
				current.Content = strings.Join(currentLines, "\n")
				chunks = append(chunks, *current)
			}

			for len(headingLevels) > 0 && headingLevels[len(headingLevels)-1] >= level {
				headingLevels = headingLevels[:len(headingLevels)-1]
				headingStack = headingStack[:len(headingStack)-1]
			}
			headingLevels = append(headingLevels, level)
			headingStack = append(headingStack, title)
			titlePath := strings.Join(headingStack, " / ")

			current = &MDChunk{
				Title:     titlePath,
				LineStart: i + 1,
			}
			currentLines = []string{line}
			continue
		}

		if current == nil {
			current = &MDChunk{
				Title:     "",
				LineStart: 1,
			}
			currentLines = []string{}
		}
		currentLines = append(currentLines, line)
	}

	if current != nil {
		current.LineEnd = len(lines)
		current.Content = strings.Join(currentLines, "\n")
		chunks = append(chunks, *current)
	}
	return splitMarkdownChunks(chunks, markdownMaxChars)
}

func parseHeading(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	if len(trimmed) > level && trimmed[level] != ' ' {
		return 0, "", false
	}
	title := strings.TrimSpace(trimmed[level:])
	return level, title, true
}

type mdSegment struct {
	lines     []string
	text      string
	startLine int
	endLine   int
}

func splitMarkdownChunks(chunks []MDChunk, maxChars int) []MDChunk {
	if maxChars <= 0 || len(chunks) == 0 {
		return chunks
	}
	var out []MDChunk
	for _, chunk := range chunks {
		if countMarkdownChars(chunk.Content) <= maxChars {
			out = append(out, chunk)
			continue
		}
		out = append(out, splitMarkdownChunk(chunk, maxChars)...)
	}
	return out
}

func splitMarkdownChunk(chunk MDChunk, maxChars int) []MDChunk {
	segments := buildMarkdownSegments(chunk, maxChars)
	if len(segments) == 0 {
		return nil
	}
	var out []MDChunk
	var currentLines []string
	currentLen := 0
	currentStart := 0
	currentEnd := 0
	for _, seg := range segments {
		segLen := countMarkdownChars(seg.text)
		if currentLen+segLen > maxChars && currentLen > 0 {
			out = append(out, MDChunk{
				Title:     chunk.Title,
				Content:   strings.Join(currentLines, "\n"),
				LineStart: currentStart,
				LineEnd:   currentEnd,
			})
			currentLines = nil
			currentLen = 0
			currentStart = 0
			currentEnd = 0
		}
		if currentLen == 0 {
			currentStart = seg.startLine
		}
		currentLines = append(currentLines, seg.lines...)
		currentLen += segLen
		currentEnd = seg.endLine
	}
	if currentLen > 0 {
		out = append(out, MDChunk{
			Title:     chunk.Title,
			Content:   strings.Join(currentLines, "\n"),
			LineStart: currentStart,
			LineEnd:   currentEnd,
		})
	}
	return out
}

func buildMarkdownSegments(chunk MDChunk, maxChars int) []mdSegment {
	lines := strings.Split(chunk.Content, "\n")
	var segments []mdSegment
	var segLines []string
	segStart := chunk.LineStart
	for i, line := range lines {
		lineNo := chunk.LineStart + i
		if len(segLines) == 0 {
			segStart = lineNo
		}
		segLines = append(segLines, line)
		if strings.TrimSpace(line) == "" {
			segments = append(segments, newSegment(segLines, segStart, lineNo))
			segLines = nil
		}
	}
	if len(segLines) > 0 {
		segments = append(segments, newSegment(segLines, segStart, chunk.LineStart+len(lines)-1))
	}

	var expanded []mdSegment
	for _, seg := range segments {
		if countMarkdownChars(seg.text) <= maxChars {
			expanded = append(expanded, seg)
			continue
		}
		expanded = append(expanded, splitSegmentByLines(seg, maxChars)...)
	}
	return expanded
}

func splitSegmentByLines(seg mdSegment, maxChars int) []mdSegment {
	var out []mdSegment
	var curLines []string
	curStart := seg.startLine
	curLen := 0
	for i, line := range seg.lines {
		lineNo := seg.startLine + i
		lineLen := countMarkdownChars(line) + 1
		if curLen+lineLen > maxChars && curLen > 0 {
			out = append(out, newSegment(curLines, curStart, lineNo-1))
			curLines = nil
			curStart = lineNo
			curLen = 0
		}
		curLines = append(curLines, line)
		curLen += lineLen
	}
	if len(curLines) > 0 {
		out = append(out, newSegment(curLines, curStart, seg.startLine+len(seg.lines)-1))
	}
	return out
}

func newSegment(lines []string, startLine, endLine int) mdSegment {
	text := strings.Join(lines, "\n")
	return mdSegment{
		lines:     lines,
		text:      text,
		startLine: startLine,
		endLine:   endLine,
	}
}

func countMarkdownChars(text string) int {
	return len([]rune(text))
}
