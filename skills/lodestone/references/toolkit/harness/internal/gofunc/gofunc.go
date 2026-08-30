package gofunc

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

type Func struct {
	File  string
	Name  string
	Line  int
	Lines []string
}

func (f Func) Key() string { return f.File + "::" + f.Name }

var generated = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// generatedHeaderLines is how deep the Go convention allows the marker.
const generatedHeaderLines = 10

func IsGenerated(src []byte) bool {
	for _, line := range bytes.Split(src, []byte("\n"))[:min(generatedHeaderLines, bytes.Count(src, []byte("\n"))+1)] {
		if generated.Match(bytes.TrimRight(line, "\r")) {
			return true
		}
	}
	return false
}

// Parse returns nothing for a file that does not parse. A syntactically broken
// revision is a fact about that commit, not a reason to abandon the walk.
func Parse(path string, src []byte) []Func {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(src), "\n")

	var funcs []Func
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		open := fset.PositionFor(function.Body.Lbrace, false).Line
		shut := fset.PositionFor(function.Body.Rbrace, false).Line
		const leastBodyLines = 2
		if shut-open < leastBodyLines {
			continue
		}

		funcs = append(funcs, Func{
			File:  path,
			Name:  name(function),
			Line:  fset.PositionFor(function.Pos(), false).Line,
			Lines: normalize(lines[open : shut-1]),
		})
	}
	return funcs
}

func name(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return recvType(function.Recv.List[0].Type) + "." + function.Name.Name
}

func recvType(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return recvType(typed.X)
	case *ast.IndexExpr:
		return recvType(typed.X)
	case *ast.IndexListExpr:
		return recvType(typed.X)
	case *ast.Ident:
		return typed.Name
	}
	return "?"
}

var space = regexp.MustCompile(`\s+`)

func normalize(raw []string) []string {
	var out []string
	for _, line := range raw {
		trimmed := space.ReplaceAllString(strings.TrimSpace(line), " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func set(lines []string) map[string]bool {
	s := make(map[string]bool, len(lines))
	for _, line := range lines {
		s[line] = true
	}
	return s
}

// Overlap is Jaccard over normalized body lines. It is deliberately lexical and
// deliberately not the similarity the tool under test computes.
func Overlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	left, right := set(a), set(b)
	shared := 0
	for line := range left {
		if right[line] {
			shared++
		}
	}
	union := len(left) + len(right) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// Contained is the share of a that also appears in b, which is what an extracted
// helper asks about: not whether two bodies match, but whether one absorbed the
// other's lines.
func Contained(inner, outer []string) float64 {
	if len(inner) == 0 {
		return 0
	}

	right := set(outer)
	shared := 0
	for line := range set(inner) {
		if right[line] {
			shared++
		}
	}
	return float64(shared) / float64(len(set(inner)))
}

func Removed(parent, child []string) []string {
	surviving := set(child)
	var gone []string
	for _, line := range parent {
		if !surviving[line] {
			gone = append(gone, line)
		}
	}
	return gone
}
