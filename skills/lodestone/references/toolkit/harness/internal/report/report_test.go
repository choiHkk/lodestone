package report

import (
	"bytes"
	"strings"
	"testing"

	"lodestone/internal/analyze"
	"lodestone/internal/fusion"
)

func TestWriteMarkdown(t *testing.T) {
	t.Parallel()

	result := Result{
		Root:                 "/repo",
		Patterns:             []string{"./..."},
		ModelProfile:         "qwen3",
		FunctionCount:        2,
		CandidateCount:       1,
		MinimumSemanticScore: 0.55,
		RRFK:                 60,
		Candidates: []fusion.Candidate{{
			Candidate: analyze.Candidate{
				Left:           analyze.FunctionRef{File: "a.go", Line: 10, Name: "read"},
				Right:          analyze.FunctionRef{File: "b.go", Line: 20, Name: "load"},
				SemanticScore:  0.9,
				StructureScore: 0.8,
				CallScore:      0.5,
			},
			RRFScore: 0.03,
			Sources:  []fusion.Source{{Name: semanticSource, Rank: 1}},
		}},
	}
	var output bytes.Buffer
	if err := Write(&output, "markdown", result); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Hybrid code-similarity", "`qwen3`", "`a.go:10`", "`b.go:20`", "0.9000", "semantic:1",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("report does not contain %q", expected)
		}
	}
}
