package bcindex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

type goListPackage struct {
	ImportPath string   `json:"ImportPath"`
	Dir        string   `json:"Dir"`
	Imports    []string `json:"Imports"`
}

type GoPackageIndex struct {
	DirToImportPath map[string]string
	Depends         []Relation
}

func BuildGoPackageIndex(root string) (*GoPackageIndex, error) {
	pkgs, err := listGoPackages(root)
	if err != nil {
		return nil, err
	}
	localPackages := make(map[string]goListPackage)
	dirToImport := make(map[string]string)
	dirForImport := make(map[string]string)

	for _, pkg := range pkgs {
		if pkg.ImportPath == "" || pkg.Dir == "" {
			continue
		}
		relDir, ok := relDirWithinRoot(root, pkg.Dir)
		if !ok {
			continue
		}
		localPackages[pkg.ImportPath] = pkg
		dirToImport[relDir] = pkg.ImportPath
		dirForImport[pkg.ImportPath] = relDir
	}

	var depends []Relation
	for _, pkg := range localPackages {
		fromDir := dirForImport[pkg.ImportPath]
		for _, imp := range pkg.Imports {
			if _, ok := localPackages[imp]; !ok {
				continue
			}
			depends = append(depends, Relation{
				FromRef:    pkg.ImportPath,
				ToRef:      imp,
				Kind:       RelationKindDependsOn,
				File:       fromDir,
				Line:       0,
				Source:     RelationSourceGoList,
				Confidence: 0.9,
			})
		}
	}

	return &GoPackageIndex{
		DirToImportPath: dirToImport,
		Depends:         depends,
	}, nil
}

func listGoPackages(root string) ([]goListPackage, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("go list: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("go list: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []goListPackage
	for {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse go list: %w", err)
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

func relDirWithinRoot(root, dir string) (string, bool) {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", false
	}
	if strings.HasPrefix(rel, "..") {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		return ".", true
	}
	return rel, true
}
