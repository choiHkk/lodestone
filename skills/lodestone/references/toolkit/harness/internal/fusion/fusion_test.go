package fusion

import (
	"testing"

	"lodestone/internal/analyze"
	"lodestone/internal/detect"
)

const (
	leftFile     = "a.go"
	rightFile    = "b.go"
	jscpdName    = "jscpd"
	duplName     = "dupl"
	semanticName = "semantic"
)

func functionPair() (analyze.Function, analyze.Function) {
	left := analyze.Function{
		ID: "a", File: leftFile, Line: 1, EndLine: 10, Lines: 10,
		Calls: []string{"read"}, NodeCounts: map[string]int{"IfStmt": 1},
	}
	right := analyze.Function{
		ID: "b", File: rightFile, Line: 1, EndLine: 10, Lines: 10,
		Calls: []string{"read"}, NodeCounts: map[string]int{"IfStmt": 1},
	}

	return left, right
}

func wholeFilePair(metric float64) detect.Pair {
	return detect.Pair{
		First:  detect.Fragment{File: leftFile, Start: 1, End: 10},
		Second: detect.Fragment{File: rightFile, Start: 1, End: 10},
		Metric: metric,
	}
}

func fuseSemanticPair(t *testing.T, retrieved []Retrieved) Result {
	t.Helper()

	left, right := functionPair()
	semantic := []analyze.Candidate{analyze.Compare(left, right, 0.9)}
	fused, err := Fuse(t.TempDir(), []analyze.Function{left, right}, semantic, retrieved, 60, 10, false)
	if err != nil {
		t.Fatal(err)
	}

	return fused
}

func TestFuseRanksAcrossSources(t *testing.T) {
	t.Parallel()

	fused := fuseSemanticPair(t, []Retrieved{
		{Name: jscpdName, Pairs: []detect.Pair{wholeFilePair(80)}},
		{Name: duplName, Pairs: []detect.Pair{wholeFilePair(10)}},
	})
	if len(fused.Candidates) != 1 {
		t.Fatalf("got %d candidates", len(fused.Candidates))
	}
	if len(fused.Candidates[0].Sources) != 3 {
		t.Fatalf("sources = %v", fused.Candidates[0].Sources)
	}
	if fused.Candidates[0].RRFScore <= 3.0/62.0 {
		t.Fatalf("rrf = %v", fused.Candidates[0].RRFScore)
	}
	for _, name := range []string{semanticName, jscpdName, duplName} {
		if fused.Counts[name] != 1 {
			t.Fatalf("counts[%s] = %d", name, fused.Counts[name])
		}
	}
}

func TestFuseKeepsEmptyRetrieverOutOfCounts(t *testing.T) {
	t.Parallel()

	fused := fuseSemanticPair(t, []Retrieved{{Name: jscpdName}, {Name: duplName}})
	if fused.Counts[semanticName] != 1 {
		t.Fatalf("semantic count = %d", fused.Counts[semanticName])
	}
	if _, ok := fused.Counts[jscpdName]; ok {
		t.Fatal("jscpd contributed without pairs")
	}
}

func TestFuseSkipsTestFilePairs(t *testing.T) {
	t.Parallel()

	left := analyze.Function{ID: "a", File: "a_test.go", Line: 1, EndLine: 10, Lines: 10}
	right := analyze.Function{ID: "b", File: rightFile, Line: 1, EndLine: 10, Lines: 10}
	retrieved := []Retrieved{{Name: jscpdName, Pairs: []detect.Pair{{
		First:  detect.Fragment{File: "a_test.go", Start: 1, End: 10},
		Second: detect.Fragment{File: rightFile, Start: 1, End: 10},
		Metric: 80,
	}}}}
	fused, err := Fuse(t.TempDir(), []analyze.Function{left, right}, nil, retrieved, 60, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fused.Candidates) != 0 {
		t.Fatalf("got %d candidates from a test-file pair", len(fused.Candidates))
	}
}
