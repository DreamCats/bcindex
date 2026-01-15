package bcindex

import "strings"

type MDChunk struct {
	Title     string
	Content   string
	LineStart int
	LineEnd   int
}

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
	return chunks
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
