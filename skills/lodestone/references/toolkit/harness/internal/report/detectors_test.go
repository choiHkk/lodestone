package report

import (
	"bytes"
	"strings"
	"testing"

	"lodestone/internal/analyze"
	"lodestone/internal/fusion"
)

const (
	semanticSource = "semantic"
	leftID         = "left"
	rightID        = "right"
	ifStatement    = "IfStmt"
)

func render(t *testing.T, result Result) string {
	t.Helper()

	var output bytes.Buffer
	if err := Write(&output, "markdown", result); err != nil {
		t.Fatal(err)
	}

	return output.String()
}

func requireAll(t *testing.T, rendered string, expected ...string) {
	t.Helper()

	for _, want := range expected {
		if !strings.Contains(rendered, want) {
			t.Fatalf("report is missing %q", want)
		}
	}
}

func TestReportSeparatesCleanRetrieverFromAbsentOne(t *testing.T) {
	t.Parallel()

	rendered := render(t, Result{
		Detectors: []Detector{
			{Name: semanticSource, Status: StatusRan, Pairs: 12},
			{Name: "jscpd", Status: StatusRan, Pairs: 0},
			{Name: "dupl", Status: StatusSkipped},
		},
	})
	requireAll(t, rendered, "`semantic` | ran | 12", "`jscpd` | ran | 0", "`dupl` | skipped")
	if !strings.Contains(rendered, "contributed no ranks") {
		t.Fatal("a skipped retriever must be called out in prose, not only in the table")
	}
}

func TestReportStaysQuietWhenEveryRetrieverRan(t *testing.T) {
	t.Parallel()

	rendered := render(t, Result{
		Detectors: []Detector{{Name: semanticSource, Status: StatusRan, Pairs: 3}},
	})
	if strings.Contains(rendered, "contributed no ranks") {
		t.Fatal("the warning appeared with no skipped or failed retriever")
	}
}

func TestReportKeepsDetectorFailureInsideOneTableCell(t *testing.T) {
	t.Parallel()

	rendered := render(t, Result{
		Detectors: []Detector{
			{Name: "jscpd", Status: StatusFailed, Detail: "run jscpd: exit 2\nstderr | broken"},
		},
	})
	for line := range strings.SplitSeq(rendered, "\n") {
		if !strings.Contains(line, "run jscpd: exit 2") {
			continue
		}
		if !strings.Contains(line, "stderr") {
			t.Fatalf("the second line of the detail was dropped: %q", line)
		}
		columns := strings.Count(strings.ReplaceAll(line, "\\|", ""), "|")
		if columns != strings.Count("| a | b | c | d |", "|") {
			t.Fatalf("detail added %d unescaped pipes to the row: %q", columns, line)
		}

		return
	}
	t.Fatal("detector detail never reached the report")
}

func TestReportNamesExcludedSource(t *testing.T) {
	t.Parallel()

	rendered := render(t, Result{
		GeneratedFiles:   []string{"internal/wire.pb.go"},
		IgnoredFunctions: []string{"internal/a.go:10:Retired"},
	})
	requireAll(
		t, rendered,
		"Skipped 1 generated files", "internal/wire.pb.go", "--include-generated",
		"Skipped 1 functions marked", analyze.IgnoreMarker, "internal/a.go:10:Retired",
	)
}

func TestReportTruncatesLongExclusionLists(t *testing.T) {
	t.Parallel()

	const total = 25
	generated := make([]string, 0, total)
	for index := range total {
		generated = append(generated, "pkg/file"+string(rune('a'+index))+".go")
	}
	rendered := render(t, Result{GeneratedFiles: generated})
	requireAll(t, rendered, "Skipped 25 generated files", "and 15 more")
}

func TestAttachEvidenceScoresSequenceAndClipsSource(t *testing.T) {
	t.Parallel()

	left := analyze.Function{
		ID: leftID, Source: "one\ntwo\nthree\nfour",
		NodeSequence: []string{ifStatement, "ReturnStmt"},
	}
	right := analyze.Function{
		ID: rightID, Source: "one\ntwo",
		NodeSequence: []string{ifStatement, "ReturnStmt"},
	}
	candidates := []fusion.Candidate{{Candidate: analyze.Candidate{
		Left:  analyze.FunctionRef{ID: leftID},
		Right: analyze.FunctionRef{ID: rightID},
	}}}
	index := map[string]analyze.Function{leftID: left, rightID: right}
	AttachEvidence(candidates, index, 2)
	if candidates[0].SequenceScore != 1 {
		t.Fatalf("identical node sequences scored %v", candidates[0].SequenceScore)
	}
	if !strings.Contains(candidates[0].LeftSource, "2 more lines") {
		t.Fatalf("long body was not clipped: %q", candidates[0].LeftSource)
	}
	if strings.Contains(candidates[0].RightSource, "more lines") {
		t.Fatalf("short body was clipped: %q", candidates[0].RightSource)
	}
}

func TestAttachEvidenceOmitsSourceWhenDisabled(t *testing.T) {
	t.Parallel()

	candidates := []fusion.Candidate{{Candidate: analyze.Candidate{
		Left:  analyze.FunctionRef{ID: leftID},
		Right: analyze.FunctionRef{ID: rightID},
	}}}
	index := map[string]analyze.Function{
		leftID:  {ID: leftID, Source: "body", NodeSequence: []string{ifStatement}},
		rightID: {ID: rightID, Source: "body", NodeSequence: []string{ifStatement}},
	}
	AttachEvidence(candidates, index, 0)
	if candidates[0].LeftSource != "" || candidates[0].RightSource != "" {
		t.Fatal("bodies were carried despite a zero cap")
	}
	if candidates[0].SequenceScore != 1 {
		t.Fatal("sequence evidence must still be scored when bodies are omitted")
	}
}

func TestWriteRejectsUnknownFormat(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := Write(&output, "yaml", Result{}); err == nil {
		t.Fatal("an unknown format should fail")
	}
}
