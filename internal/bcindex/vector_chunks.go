package bcindex

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

func BuildGoVectorChunks(file string, content []byte, maxChars int, overlap int) []VectorChunk {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, content, parser.ParseComments)
	if err != nil {
		return nil
	}
	tokenFile := fset.File(parsed.Pos())
	if tokenFile == nil {
		return nil
	}
	var out []VectorChunk
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		startPos := fn.Pos()
		if fn.Doc != nil {
			startPos = fn.Doc.Pos()
		}
		endPos := fn.End()
		startOff := tokenFile.Offset(startPos)
		endOff := tokenFile.Offset(endPos)
		if startOff < 0 || startOff >= len(content) || endOff <= startOff {
			continue
		}
		if endOff > len(content) {
			endOff = len(content)
		}
		kind := "go_func"
		name := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv := typeString(fset, fn.Recv.List[0].Type)
			if recv != "" {
				kind = "go_method"
				name = recv + "." + name
			}
		}
		startLine := fset.Position(startPos).Line
		endLine := fset.Position(endPos).Line
		text := strings.TrimSpace(string(content[startOff:endOff]))
		if text == "" {
			continue
		}
		if maxChars > 0 && countChars(text) > maxChars {
			subs := splitTextByChars(text, maxChars, overlap)
			for i, sub := range subs {
				chunkID := fmt.Sprintf("%s#%d", name, i+1)
				hash := sha1.Sum([]byte(file + ":" + chunkID + ":" + sub))
				hashHex := hex.EncodeToString(hash[:])
				out = append(out, VectorChunk{
					ID:        "vec:" + file + ":" + hashHex,
					File:      file,
					Kind:      kind,
					Name:      name,
					Title:     chunkID,
					Text:      sub,
					LineStart: startLine,
					LineEnd:   endLine,
					Hash:      hashHex,
				})
			}
			continue
		}
		hash := sha1.Sum([]byte(file + ":" + name + ":" + text))
		hashHex := hex.EncodeToString(hash[:])
		out = append(out, VectorChunk{
			ID:        "vec:" + file + ":" + hashHex,
			File:      file,
			Kind:      kind,
			Name:      name,
			Title:     name,
			Text:      text,
			LineStart: startLine,
			LineEnd:   endLine,
			Hash:      hashHex,
		})
	}
	return out
}

func BuildMarkdownVectorChunks(file string, chunks []MDChunk, maxChars int) []VectorChunk {
	var out []VectorChunk
	for _, chunk := range chunks {
		text := strings.TrimSpace(chunk.Content)
		text = truncateText(text, maxChars)
		if text == "" {
			continue
		}
		hash := sha1.Sum([]byte(file + ":" + chunk.Title + ":" + text))
		hashHex := hex.EncodeToString(hash[:])
		out = append(out, VectorChunk{
			ID:        "vec:" + file + ":" + hashHex,
			File:      file,
			Kind:      "md_section",
			Title:     chunk.Title,
			Text:      text,
			LineStart: chunk.LineStart,
			LineEnd:   chunk.LineEnd,
			Hash:      hashHex,
		})
	}
	return out
}

func truncateText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars])
}

func splitTextByChars(text string, maxChars int, overlap int) []string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return []string{text}
	}
	if overlap < 0 {
		overlap = 0
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return []string{text}
	}
	step := maxChars - overlap
	if step <= 0 {
		step = maxChars
	}
	var out []string
	for start := 0; start < len(runes); start += step {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}

func countChars(text string) int {
	return len([]rune(text))
}
