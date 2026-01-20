package docgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
)

// Writer writes generated documentation back to source files
type Writer struct {
	dryRun  bool
	gofmt   bool
	diff    bool
	verbose bool
}

// NewWriter creates a new writer
func NewWriter(opts ...WriterOption) *Writer {
	w := &Writer{
		dryRun: false,
		gofmt:  true,
		diff:   false,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// WriterOption configures the writer
type WriterOption func(*Writer)

// WithDryRun sets dry run mode (no actual file modifications)
func WithDryRun(dryRun bool) WriterOption {
	return func(w *Writer) {
		w.dryRun = dryRun
	}
}

// WithGofmt sets whether to gofmt files after writing
func WithGofmt(gofmt bool) WriterOption {
	return func(w *Writer) {
		w.gofmt = gofmt
	}
}

// WithDiff sets whether to output diff instead of writing
func WithDiff(diff bool) WriterOption {
	return func(w *Writer) {
		w.diff = diff
	}
}

// WithVerbose sets verbose output
func WithVerbose(verbose bool) WriterOption {
	return func(w *Writer) {
		w.verbose = verbose
	}
}

// WriteResult represents the result of writing documentation
type WriteResult struct {
	File     string
	Symbol   string
	Success  bool
	Error    string
	Diff     string // If diff mode
	Modified bool   // If file was modified
}

// WriteRequest represents a request to write documentation
type WriteRequest struct {
	File      string
	Symbol    string
	Line      int    // Line number of the symbol declaration
	Comment   string // Documentation comment to insert
	Overwrite bool   // Whether to overwrite existing comments
}

// Write writes documentation to files
func (w *Writer) Write(requests []WriteRequest) []WriteResult {
	var results []WriteResult

	// Group requests by file
	fileRequests := make(map[string][]WriteRequest)
	for _, req := range requests {
		fileRequests[req.File] = append(fileRequests[req.File], req)
	}

	// Process each file
	for filePath, reqs := range fileRequests {
		fileResults := w.writeFile(filePath, reqs)
		results = append(results, fileResults...)
	}

	return results
}

// writeFile writes documentation to a single file
func (w *Writer) writeFile(filePath string, requests []WriteRequest) []WriteResult {
	var results []WriteResult

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		for _, req := range requests {
			results = append(results, WriteResult{
				File:    filePath,
				Symbol:  req.Symbol,
				Success: false,
				Error:   fmt.Sprintf("failed to read file: %v", err),
			})
		}
		return results
	}

	// Parse the file to find AST positions
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		for _, req := range requests {
			results = append(results, WriteResult{
				File:    filePath,
				Symbol:  req.Symbol,
				Success: false,
				Error:   fmt.Sprintf("failed to parse file: %v", err),
			})
		}
		return results
	}

	// Build line to position mapping
	originalLines := bytes.Split(content, []byte{'\n'})

	// Use a pointer to lines so we can modify it
	lines := originalLines

	// Track all modifications needed
	type modification struct {
		insertLine int
		comment    [][]byte
		overwrite  bool
	}
	var modifications []modification

	// First pass: collect all modifications
	for _, req := range requests {
		result := w.collectModification(node, fset, lines, filePath, req)
		results = append(results, result.result)

		if result.result.Success && result.result.Modified && !w.dryRun && !w.diff {
			modifications = append(modifications, modification{
				insertLine: result.insertLine,
				comment:    result.commentBytes,
				overwrite:  req.Overwrite,
			})
		}
	}

	// Apply all modifications at once (in reverse order to maintain line numbers)
	if !w.dryRun && !w.diff && len(modifications) > 0 {
		// Sort modifications by insertLine in descending order
		for i := 0; i < len(modifications); i++ {
			for j := i + 1; j < len(modifications); j++ {
				if modifications[i].insertLine < modifications[j].insertLine {
					modifications[i], modifications[j] = modifications[j], modifications[i]
				}
			}
		}

		// Apply modifications
		for _, mod := range modifications {
			if mod.overwrite {
				// Skip existing comment lines
				i := mod.insertLine
				for i < len(lines) && (bytes.HasPrefix(lines[i], []byte("//")) || bytes.HasPrefix(bytes.TrimSpace(lines[i]), []byte("/*"))) {
					i++
				}
				lines = append(lines[:mod.insertLine], append(mod.comment, lines[i:]...)...)
			} else {
				lines = append(lines[:mod.insertLine+1], append(mod.comment, lines[mod.insertLine+1:]...)...)
			}
		}

		// Write back the file
		newContent := bytes.Join(lines, []byte{'\n'})
		if err := os.WriteFile(filePath, newContent, 0644); err != nil {
			// Mark all as error
			for i := range results {
				if results[i].Success {
					results[i].Success = false
					results[i].Error = fmt.Sprintf("failed to write file: %v", err)
				}
			}
			return results
		}
	}

	return results
}

// collectModification collects modification info for a single write request
type collectResult struct {
	result       WriteResult
	insertLine   int
	commentBytes [][]byte
}

func (w *Writer) collectModification(node *ast.File, fset *token.FileSet, lines [][]byte, filePath string, req WriteRequest) collectResult {
	result := WriteResult{
		File:    filePath,
		Symbol:  req.Symbol,
		Success: false,
	}

	// Find the declaration
	var decl ast.Decl
	var declLine int

	found := false
	for _, d := range node.Decls {
		line := fset.Position(d.Pos()).Line
		if line == req.Line {
			decl = d
			declLine = line
			found = true
			break
		}
	}

	if !found {
		result.Error = fmt.Sprintf("declaration not found at line %d", req.Line)
		return collectResult{result: result}
	}

	// Check if there's already a doc comment
	hasDoc := false
	switch d := decl.(type) {
	case *ast.FuncDecl:
		hasDoc = d.Doc != nil && len(d.Doc.List) > 0
	case *ast.GenDecl:
		// For GenDecl, check both decl.Doc and spec.Doc
		// decl.Doc applies to the whole declaration group
		// spec.Doc applies to individual type specs
		hasDoc = d.Doc != nil && len(d.Doc.List) > 0
		// Also check each spec's doc
		if !hasDoc {
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if ts.Doc != nil && len(ts.Doc.List) > 0 {
						// Check if this spec is at the requested line
						specLine := fset.Position(ts.Pos()).Line
						if specLine == req.Line {
							hasDoc = true
							break
						}
					}
				}
			}
		}
	}

	if hasDoc && !req.Overwrite {
		result.Error = "symbol already has documentation (use --overwrite to replace)"
		return collectResult{result: result}
	}

	// Build the comment
	commentLines := formatComment(req.Comment)

	// Convert comment lines to []byte
	commentBytes := make([][]byte, len(commentLines))
	for i, line := range commentLines {
		commentBytes[i] = []byte(line)
	}

	// Insert the comment before the declaration
	insertLine := declLine - 2 // Convert to 0-indexed

	// In diff mode, generate diff
	if w.diff {
		result.Diff = generateDiff(lines, insertLine, commentLines)
		result.Success = true
		return collectResult{result: result, insertLine: insertLine, commentBytes: commentBytes}
	}

	// In dry-run mode, just report what would be done
	if w.dryRun {
		if w.verbose {
			fmt.Printf("Would insert comment at %s:%d\n%s\n", filePath, req.Line, strings.Join(commentLines, "\n"))
		}
		result.Success = true
		result.Modified = false
		return collectResult{result: result, insertLine: insertLine, commentBytes: commentBytes}
	}

	result.Success = true
	result.Modified = true
	return collectResult{result: result, insertLine: insertLine, commentBytes: commentBytes}
}

// formatComment formats a documentation comment
func formatComment(comment string) []string {
	lines := strings.Split(comment, "\n")
	result := make([]string, 0, len(lines))

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if i < len(lines)-1 { // Don't add trailing empty line
				result = append(result, "//")
			}
		} else {
			result = append(result, "// "+line)
		}
	}

	return result
}

// generateDiff generates a unified diff for the change
func generateDiff(lines [][]byte, insertLine int, commentLines []string) string {
	var buf bytes.Buffer

	// Get a few lines of context
	contextStart := insertLine - 2
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := insertLine + 3
	if contextEnd > len(lines) {
		contextEnd = len(lines)
	}

	buf.WriteString("--- <original>\n")
	buf.WriteString("+++ <modified>\n")
	buf.WriteString("@@")
	// Simplified hunk header
	buf.WriteString(fmt.Sprintf(" -%d,%d +%d,%d @@\n", contextStart+1, contextEnd-contextStart,
		contextStart+1, contextEnd-contextStart+len(commentLines)))

	// Output context before
	for i := contextStart; i < insertLine+1 && i < len(lines); i++ {
		buf.WriteString(" " + string(lines[i]) + "\n")
	}

	// Output removal (if replacing existing comment)
	// For now, just output additions

	// Output additions
	for _, line := range commentLines {
		buf.WriteString("+" + line + "\n")
	}

	// Output context after
	for i := insertLine + 1; i < contextEnd && i < len(lines); i++ {
		buf.WriteString(" " + string(lines[i]) + "\n")
	}

	return buf.String()
}

// PrintFile prints the AST of a file (for debugging)
func (w *Writer) PrintFile(filePath string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	return printer.Fprint(os.Stdout, fset, node)
}
