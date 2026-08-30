package analyze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

var (
	errEmbeddingCount = errors.New("embedding count does not match function count")
	errLoadPackages   = errors.New("load Go packages")
)

const (
	chunkOverlapDivisor  = 2
	maximumSequenceNodes = 4096

	IgnoreMarker    = "similarity:ignore"
	directivePrefix = "//go:"

	LocalityCrossPackage = "cross-package"
	LocalitySamePackage  = "same-package"
	LocalitySameFile     = "same-file"
)

func LocalityRank(locality string) int {
	order := []string{LocalityCrossPackage, LocalitySamePackage, LocalitySameFile}
	if index := slices.Index(order, locality); index >= 0 {
		return index
	}

	return len(order)
}

type Function struct {
	ID         string         `json:"id"`
	Package    string         `json:"package"`
	Name       string         `json:"name"`
	File       string         `json:"file"`
	Line       int            `json:"line"`
	EndLine    int            `json:"endLine"`
	Lines      int            `json:"lines"`
	Source     string         `json:"-"`
	Calls      []string       `json:"calls,omitempty"`
	NodeCounts map[string]int `json:"-"`

	NodeSequence []string `json:"-"`
}

type FunctionRef = Function

type Candidate struct {
	Left           FunctionRef `json:"left"`
	Right          FunctionRef `json:"right"`
	SemanticScore  float64     `json:"semanticScore"`
	Separation     float64     `json:"separation"`
	StructureScore float64     `json:"structureScore"`
	SequenceScore  float64     `json:"sequenceScore"`
	SizeRatio      float64     `json:"sizeRatio"`
	CallScore      float64     `json:"callScore"`
	Locality       string      `json:"locality"`
	SharedCalls    []string    `json:"sharedCalls,omitempty"`
	LeftOnlyCalls  []string    `json:"leftOnlyCalls,omitempty"`
	RightOnlyCalls []string    `json:"rightOnlyCalls,omitempty"`
}

type ScanOptions struct {
	Root             string
	Patterns         []string
	MinLines         int
	MinNodes         int
	IncludeTests     bool
	IncludeGenerated bool
}

type ScanResult struct {
	Functions []Function

	Generated []string

	Ignored []string
}

func Scan(ctx context.Context, settings ScanOptions) (ScanResult, error) {
	loaded, err := packages.Load(&packages.Config{
		Context: ctx,
		Dir:     settings.Root,
		Mode:    packages.NeedName | packages.NeedFiles,
		Tests:   settings.IncludeTests,
	}, settings.Patterns...)
	if err != nil {
		return ScanResult{}, fmt.Errorf("%w: %w", errLoadPackages, err)
	}
	if err := packageErrors(loaded); err != nil {
		return ScanResult{}, err
	}
	repository, err := os.OpenRoot(settings.Root)
	if err != nil {
		return ScanResult{}, fmt.Errorf("open repository: %w", err)
	}
	defer func() { _ = repository.Close() }()
	result, err := scanPackages(repository, loaded, settings)
	if err != nil {
		return ScanResult{}, err
	}
	sort.Slice(result.Functions, func(left, right int) bool {
		return result.Functions[left].ID < result.Functions[right].ID
	})
	sort.Strings(result.Generated)
	sort.Strings(result.Ignored)

	return result, nil
}

func scanPackages(
	repository *os.Root,
	loaded []*packages.Package,
	settings ScanOptions,
) (ScanResult, error) {
	seen := make(map[string]struct{})
	var result ScanResult
	for _, loadedPackage := range loaded {
		for _, path := range loadedPackage.GoFiles {
			relative, err := filepath.Rel(settings.Root, path)
			if err != nil {
				return ScanResult{}, fmt.Errorf("resolve %s: %w", path, err)
			}
			if !filepath.IsLocal(relative) {
				continue
			}
			relative = filepath.ToSlash(relative)
			if _, ok := seen[relative]; ok {
				continue
			}
			seen[relative] = struct{}{}
			found, err := scanFile(repository, loadedPackage.PkgPath, relative, settings)
			if err != nil {
				return ScanResult{}, err
			}
			if found.generated {
				result.Generated = append(result.Generated, relative)

				continue
			}
			result.Functions = append(result.Functions, found.functions...)
			result.Ignored = append(result.Ignored, found.ignored...)
		}
	}

	return result, nil
}

func packageErrors(loaded []*packages.Package) error {
	var messages []string
	for _, loadedPackage := range loaded {
		for _, packageError := range loadedPackage.Errors {
			messages = append(messages, packageError.Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", errLoadPackages, strings.Join(messages, "; "))
}

type ranker struct {
	functions  []Function
	embeddings [][]float32
	minScore   float64
}

const (
	candidatesPerFunction = 10

	// A neighbour is measured against the one behind it, so a list of one has
	// nothing to measure.
	minimumNeighboursToSeparate = 2
)

type neighbour struct {
	other      int
	score      float64
	separation float64
}

type placed struct {
	left, right int
	position    int
	score       float64
	separation  float64
}

func Rank(functions []Function, embeddings [][]float32, minScore float64, limit int) ([]Candidate, error) {
	if len(functions) != len(embeddings) {
		return nil, errEmbeddingCount
	}
	work := ranker{functions: functions, embeddings: embeddings, minScore: minScore}
	candidates := work.bestPerFunction()
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (work ranker) rankRows() [][]neighbour {
	rows := make([][]neighbour, len(work.functions))
	workers := max(min(runtime.GOMAXPROCS(0), len(work.functions)), 1)
	queue := make(chan int)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			for left := range queue {
				rows[left] = work.rankRow(left)
			}
		})
	}
	for left := range work.functions {
		queue <- left
	}
	close(queue)
	group.Wait()

	return rows
}

func (work ranker) rankRow(left int) []neighbour {
	row := make([]neighbour, 0, len(work.functions)-1)
	for right := range work.functions {
		if right == left {
			continue
		}
		row = append(row, neighbour{other: right, score: dot(work.embeddings[left], work.embeddings[right])})
	}
	sortNeighbours(row)
	// Separation describes a neighbour against this function's whole
	// distribution, so it is measured before the score floor removes the tail
	// that gives the distribution its shape.
	separate(row)

	kept := make([]neighbour, 0, candidatesPerFunction)
	for _, near := range row {
		if len(kept) == candidatesPerFunction {
			break
		}
		if near.score >= work.minScore {
			kept = append(kept, near)
		}
	}

	return kept
}

// separate measures how far each neighbour stands above the next one in the
// same function's list, in that list's own standard deviations. A pair whose
// score barely clears the one behind it sits on a plateau; a wide gap is the
// cliff that separates a candidate from the corpus around it.
func separate(row []neighbour) {
	if len(row) < minimumNeighboursToSeparate {
		return
	}
	deviation := spread(row)
	for index := range row[:len(row)-1] {
		row[index].separation = (row[index].score - row[index+1].score) / deviation
	}
}

func spread(row []neighbour) float64 {
	var total float64
	for _, near := range row {
		total += near.score
	}
	mean := total / float64(len(row))

	var variance float64
	for _, near := range row {
		variance += (near.score - mean) * (near.score - mean)
	}
	deviation := math.Sqrt(variance / float64(len(row)))
	if deviation == 0 {
		return 1
	}

	return deviation
}

func (work ranker) bestPerFunction() []Candidate {
	positions := make(map[[2]int]placed)
	for left, row := range work.rankRows() {
		for offset, near := range row {
			key := [2]int{min(left, near.other), max(left, near.other)}
			position := offset + 1
			if held, seen := positions[key]; seen && held.position <= position {
				continue
			}
			positions[key] = placed{
				left: key[0], right: key[1], position: position,
				score: near.score, separation: near.separation,
			}
		}
	}

	ordered := slices.Collect(maps.Values(positions))
	sortPlaced(ordered, work.functions)

	candidates := make([]Candidate, 0, len(ordered))
	for _, pair := range ordered {
		candidate := Compare(work.functions[pair.left], work.functions[pair.right], pair.score)
		candidate.Separation = pair.separation
		candidates = append(candidates, candidate)
	}

	return candidates
}

func sortNeighbours(row []neighbour) {
	sort.Slice(row, func(left, right int) bool {
		if row[left].score != row[right].score {
			return row[left].score > row[right].score
		}

		return row[left].other < row[right].other
	})
}

func sortPlaced(pairs []placed, functions []Function) {
	sort.Slice(pairs, func(left, right int) bool {
		if pairs[left].position != pairs[right].position {
			return pairs[left].position < pairs[right].position
		}
		if pairs[left].score != pairs[right].score {
			return pairs[left].score > pairs[right].score
		}

		return functions[pairs[left].left].ID+"\x00"+functions[pairs[left].right].ID <
			functions[pairs[right].left].ID+"\x00"+functions[pairs[right].right].ID
	})
}

func Compare(left, right Function, semanticScore float64) Candidate {
	if left.ID > right.ID {
		left, right = right, left
	}
	shared, leftOnly, rightOnly := compareSets(left.Calls, right.Calls)
	return Candidate{
		Left:           reference(left),
		Right:          reference(right),
		SemanticScore:  semanticScore,
		StructureScore: histogramCosine(left.NodeCounts, right.NodeCounts),
		CallScore:      setScore(shared, leftOnly, rightOnly),
		Locality:       locality(left, right),
		SizeRatio:      sizeRatio(left, right),
		SharedCalls:    shared,
		LeftOnlyCalls:  leftOnly,
		RightOnlyCalls: rightOnly,
	}
}

func sizeRatio(left, right Function) float64 {
	smaller := min(len(left.NodeSequence), len(right.NodeSequence))
	larger := max(len(left.NodeSequence), len(right.NodeSequence))
	if larger == 0 {
		return 0
	}

	return float64(smaller) / float64(larger)
}

func locality(left, right Function) string {
	if left.File == right.File {
		return LocalitySameFile
	}
	if left.Package == right.Package && filepath.Dir(left.File) == filepath.Dir(right.File) {
		return LocalitySamePackage
	}

	return LocalityCrossPackage
}

func TokenChunks(source string, size, splitAfter int) []string {
	if size <= 0 {
		return []string{source}
	}
	files := token.NewFileSet()
	file := files.AddFile("", -1, len(source))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(source), nil, 0)
	var offsets []int
	for {
		position, item, _ := lexer.Scan()
		if item == token.EOF {
			break
		}
		offsets = append(offsets, file.Offset(position))
	}
	threshold := splitAfter
	if threshold <= 0 {
		threshold = size
	}
	if len(offsets) <= threshold {
		return []string{source}
	}
	stride := max(1, size/chunkOverlapDivisor)
	chunks := make([]string, 0, (len(offsets)+stride-1)/stride)
	for start := 0; start < len(offsets); start += stride {
		end := min(start+size, len(offsets))
		endOffset := len(source)
		if end < len(offsets) {
			endOffset = offsets[end]
		}
		chunk := strings.TrimSpace(source[offsets[start]:endOffset])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(offsets) {
			break
		}
	}
	return chunks
}

func scanFile(
	repository *os.Root,
	packagePath, path string,
	settings ScanOptions,
) (fileScan, error) {
	source, err := repository.ReadFile(path)
	if err != nil {
		return fileScan{}, fmt.Errorf("read %s: %w", path, err)
	}
	files := token.NewFileSet()

	parsed, err := parser.ParseFile(files, path, source, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		return fileScan{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if !settings.IncludeGenerated && generated(parsed) {
		return fileScan{generated: true}, nil
	}
	var result fileScan
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		scanFunction(&result, files, source, parsed, packagePath, path, function, settings)
	}

	return result, nil
}

func scanFunction(
	result *fileScan,
	files *token.FileSet,
	source []byte,
	parsed *ast.File,
	packagePath, path string,
	function *ast.FuncDecl,
	settings ScanOptions,
) {
	start := files.PositionFor(function.Pos(), false)
	end := files.PositionFor(function.End(), false)
	lines := end.Line - start.Line + 1
	if lines < settings.MinLines {
		return
	}
	name := functionName(files, function)
	identifier := fmt.Sprintf("%s:%d:%s", path, start.Line, name)
	if ignored(function) {
		result.ignored = append(result.ignored, identifier)

		return
	}
	calls, nodes, sequence := features(function.Body)
	if len(sequence) < settings.MinNodes {
		return
	}
	result.functions = append(result.functions, Function{
		ID:           identifier,
		Package:      packagePath,
		Name:         name,
		File:         path,
		Line:         start.Line,
		EndLine:      end.Line,
		Lines:        lines,
		Source:       functionText(files.File(function.Pos()), source, parsed.Comments, function),
		Calls:        calls,
		NodeCounts:   nodes,
		NodeSequence: sequence,
	})
}

type fileScan struct {
	functions []Function
	ignored   []string
	generated bool
}

// IsGeneratedFile applies the scanner's generated-file test: the standard
// header plus the looser markers some generators emit. Evaluation tooling
// must match it or its corpus diverges from what the adapter scans.
func IsGeneratedFile(parsed *ast.File) bool {
	return generated(parsed)
}

func generated(parsed *ast.File) bool {
	if ast.IsGenerated(parsed) {
		return true
	}
	for _, group := range parsed.Comments {
		if group.Pos() > parsed.Package {
			break
		}
		text := strings.ToLower(group.Text())
		for _, marker := range []string{"code generated", "do not edit", "autogenerated file"} {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}

	return false
}

// EmbeddedText is the adapter's per-function embedding text: comments
// stripped except directives, blank runs collapsed. The adapter may still
// chunk it and prepend the configured instruction before inference.
func EmbeddedText(
	file *token.File,
	source []byte,
	comments []*ast.CommentGroup,
	function *ast.FuncDecl,
) string {
	return functionText(file, source, comments, function)
}

func functionText(
	file *token.File,
	source []byte,
	comments []*ast.CommentGroup,
	function *ast.FuncDecl,
) string {
	start := file.Offset(function.Pos())
	end := file.Offset(function.End())
	var builder strings.Builder
	cursor := start
	for _, group := range comments {
		for _, comment := range group.List {
			if comment.Pos() < function.Pos() || comment.End() > function.End() {
				continue
			}
			if strings.HasPrefix(comment.Text, directivePrefix) {
				continue
			}
			at := file.Offset(comment.Pos())
			if at < cursor {
				continue
			}
			builder.Write(source[cursor:at])
			cursor = file.Offset(comment.End())
		}
	}
	builder.Write(source[cursor:end])

	return collapseBlankLines(builder.String())
}

func collapseBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		kept = append(kept, line)
	}

	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func ignored(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(comment.Text, IgnoreMarker) {
			return true
		}
	}

	return false
}

func functionName(files *token.FileSet, function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	var receiver bytes.Buffer
	if err := format.Node(&receiver, files, function.Recv.List[0].Type); err != nil {
		return function.Name.Name
	}
	return receiver.String() + "." + function.Name.Name
}

func features(body *ast.BlockStmt) ([]string, map[string]int, []string) {
	calls := make(map[string]struct{})
	nodes := make(map[string]int)
	var sequence []string
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if _, ok := node.(ast.Stmt); ok {
			typeName := reflect.TypeOf(node).Elem().Name()
			nodes[typeName]++
			sequence = append(sequence, typeName)
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if name := expressionName(call.Fun); name != "" {
				calls[name] = struct{}{}
			}
		}

		return true
	})

	return sortedKeys(calls), nodes, sequence
}

func SequenceSimilarity(left, right Function) float64 {
	first, second := left.NodeSequence, right.NodeSequence
	if len(first) == 0 && len(second) == 0 {
		return 0
	}
	longest := max(len(first), len(second))
	if longest > maximumSequenceNodes {
		return 0
	}
	distance := editDistance(first, second)

	return 1 - float64(distance)/float64(longest)
}

func editDistance(first, second []string) int {
	if len(first) < len(second) {
		first, second = second, first
	}
	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for index := range previous {
		previous[index] = index
	}
	for outer := 1; outer <= len(first); outer++ {
		current[0] = outer
		for inner := 1; inner <= len(second); inner++ {
			substitution := previous[inner-1]
			if first[outer-1] != second[inner-1] {
				substitution++
			}
			current[inner] = min(substitution, min(previous[inner]+1, current[inner-1]+1))
		}
		previous, current = current, previous
	}

	return previous[len(second)]
}

func expressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := expressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.IndexExpr:
		return expressionName(value.X)
	case *ast.IndexListExpr:
		return expressionName(value.X)
	case *ast.ParenExpr:
		return expressionName(value.X)
	default:
		return ""
	}
}

func reference(function Function) FunctionRef {
	return FunctionRef{
		ID: function.ID, Package: function.Package, Name: function.Name,
		File: function.File, Line: function.Line, EndLine: function.EndLine,
		Lines: function.Lines, Calls: function.Calls,
	}
}

func dot(left, right []float32) float64 {
	if len(left) != len(right) {
		return -1
	}
	var total float64
	for index := range left {
		total += float64(left[index]) * float64(right[index])
	}
	return total
}

func histogramCosine(left, right map[string]int) float64 {
	var product, leftNorm, rightNorm float64
	for key, count := range left {
		leftValue := float64(count)
		product += leftValue * float64(right[key])
		leftNorm += leftValue * leftValue
	}
	for _, count := range right {
		rightValue := float64(count)
		rightNorm += rightValue * rightValue
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return product / math.Sqrt(leftNorm*rightNorm)
}

func compareSets(left, right []string) ([]string, []string, []string) {
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, item := range left {
		leftSet[item] = struct{}{}
	}
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	var shared, leftOnly, rightOnly []string
	for _, item := range left {
		if _, ok := rightSet[item]; ok {
			shared = append(shared, item)
		} else {
			leftOnly = append(leftOnly, item)
		}
	}
	for _, item := range right {
		if _, ok := leftSet[item]; !ok {
			rightOnly = append(rightOnly, item)
		}
	}
	return shared, leftOnly, rightOnly
}

func setScore(shared, leftOnly, rightOnly []string) float64 {
	total := len(shared) + len(leftOnly) + len(rightOnly)
	if total == 0 {
		return 0
	}
	return float64(len(shared)) / float64(total)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
