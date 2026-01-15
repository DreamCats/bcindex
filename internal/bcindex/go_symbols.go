package bcindex

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

func ExtractGoSymbols(rel string, src []byte) ([]Symbol, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var symbols []Symbol
	pkg := ""
	if file.Name != nil {
		pkg = file.Name.Name
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := "func"
			recv := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = "method"
				recv = typeString(fset, d.Recv.List[0].Type)
			}
			line := fset.Position(d.Pos()).Line
			symbols = append(symbols, Symbol{
				Name: d.Name.Name,
				Kind: kind,
				File: rel,
				Line: line,
				Pkg:  pkg,
				Recv: recv,
				Doc:  docSummary(d.Doc),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := "type"
					switch s.Type.(type) {
					case *ast.InterfaceType:
						kind = "interface"
					case *ast.StructType:
						kind = "struct"
					}
					line := fset.Position(s.Pos()).Line
					symbols = append(symbols, Symbol{
						Name: s.Name.Name,
						Kind: kind,
						File: rel,
						Line: line,
						Pkg:  pkg,
						Doc:  docSummary(d.Doc),
					})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						line := fset.Position(name.Pos()).Line
						symbols = append(symbols, Symbol{
							Name: name.Name,
							Kind: kind,
							File: rel,
							Line: line,
							Pkg:  pkg,
							Doc:  docSummary(d.Doc),
						})
					}
				}
			}
		}
	}

	return symbols, nil
}

func typeString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, expr)
	return buf.String()
}

func docSummary(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := strings.TrimSpace(cg.Text())
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	return strings.TrimSpace(lines[0])
}
