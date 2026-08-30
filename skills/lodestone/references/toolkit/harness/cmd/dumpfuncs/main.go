package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"lodestone/internal/analyze"
)

type unit struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Source string `json:"source"`
}

const defaultMinLines = 6

func main() {
	root := flag.String("repo", "", "repository root")
	minLines := flag.Int("min-lines", defaultMinLines, "shortest body considered")
	flag.Parse()
	if *root == "" {
		fmt.Fprintln(os.Stderr, "dumpfuncs: -repo is required")
		os.Exit(1)
	}

	units, err := walk(*root, *minLines)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dumpfuncs:", err)
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	for _, u := range units {
		if err := encoder.Encode(u); err != nil {
			fmt.Fprintln(os.Stderr, "dumpfuncs:", err)
			os.Exit(1)
		}
	}
	fmt.Fprintf(os.Stderr, "%d units\n", len(units))
}

func walk(root string, minLines int) ([]unit, error) {
	var units []unit
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		src, err := os.ReadFile(path) //nolint:gosec // the tree walked comes from the operator's own flag
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		units = append(units, fileUnits(root, path, src, minLines)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return units, nil
}

// fileUnits returns nothing for a file that does not parse: this dumps what
// can be embedded, it is not a compiler.
func fileUnits(root, path string, src []byte, minLines int) []unit {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	if analyze.IsGeneratedFile(file) {
		return nil
	}

	var units []unit
	rel, _ := filepath.Rel(root, path)
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := fset.PositionFor(function.Pos(), false).Line
		end := fset.PositionFor(function.End(), false).Line
		if end-start+1 < minLines {
			continue
		}
		units = append(units, unit{
			Name: name(function),
			File: rel,
			Line: start,
			// The same representation the adapter embeds, so rank.py
			// measures the shipped pipeline rather than raw source.
			Source: analyze.EmbeddedText(fset.File(file.Pos()), src, file.Comments, function),
		})
	}
	return units
}

func name(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return recv(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func recv(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		// The analyzer names methods with the receiver as written,
		// star included; the dump must speak the same dialect.
		return "*" + recv(typed.X)
	case *ast.IndexExpr:
		return recv(typed.X)
	case *ast.IndexListExpr:
		return recv(typed.X)
	case *ast.Ident:
		return typed.Name
	}
	return "?"
}
