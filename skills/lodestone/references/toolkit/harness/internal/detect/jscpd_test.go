package detect

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func writeReport(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), jscpdOutputName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func readReport(t *testing.T, body string) []Pair {
	t.Helper()

	pairs, err := ReadJSCPD(writeReport(t, body))
	if err != nil {
		t.Fatal(err)
	}

	return pairs
}

func duplicateJSON(first, second, format string, tokens int) string {
	return `{"firstFile":{"name":"` + first + `","start":1,"end":20},` +
		`"secondFile":{"name":"` + second + `","start":1,"end":20},` +
		`"format":"` + format + `","lines":20,"tokens":` + strconv.Itoa(tokens) + `}`
}

func TestReadJSCPDKeepsWholePaths(t *testing.T) {
	t.Parallel()

	pairs := readReport(t, `{"duplicates":[`+
		duplicateJSON("/repo/alpha/a.go", "/repo/beta/b.go", "go", 85)+`]}`)
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0].First.File != "/repo/alpha/a.go" || pairs[0].Second.File != "/repo/beta/b.go" {
		t.Fatalf("paths were not preserved: %+v", pairs[0])
	}
	if pairs[0].First.Start != 1 || pairs[0].First.End != 20 {
		t.Fatalf("line range was not preserved: %+v", pairs[0].First)
	}
	if pairs[0].Metric != 85 {
		t.Fatalf("metric = %v, want 85", pairs[0].Metric)
	}
}

func TestJSCPDAsksForAbsolutePaths(t *testing.T) {
	t.Parallel()

	arguments := jscpdArguments("/work/jscpd", Options{
		MinLines: 5, MinTokens: 50, Concurrency: 2,
		Directories: []string{alphaDir},
	})
	if !slices.Contains(arguments, "--absolute") {
		t.Fatal(
			"jscpd must run with --absolute: without it a report given subdirectories " +
				"names files by basename alone, every pair silently fails to map onto a " +
				"function, and the run still reports success with zero pairs",
		)
	}
	for _, expected := range []string{"--format", "go", "--min-lines", "5", "--min-tokens", "50"} {
		if !slices.Contains(arguments, expected) {
			t.Fatalf("arguments missing %q: %v", expected, arguments)
		}
	}
	if arguments[len(arguments)-1] != alphaDir {
		t.Fatalf("scan directories must come last: %v", arguments)
	}
}

func TestReadJSCPDDropsOtherLanguages(t *testing.T) {
	t.Parallel()

	pairs := readReport(t, `{"duplicates":[`+
		duplicateJSON("/repo/a.py", "/repo/b.py", "python", 90)+`,`+
		duplicateJSON("/repo/a.go", "/repo/b.go", "go", 60)+`]}`)
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want only the Go pair", len(pairs))
	}
	if pairs[0].First.File != "/repo/a.go" {
		t.Fatalf("kept the wrong pair: %+v", pairs[0])
	}
}

func TestReadJSCPDRanksLargestCloneFirst(t *testing.T) {
	t.Parallel()

	pairs := readReport(t, `{"duplicates":[`+
		duplicateJSON("/repo/small/a.go", "/repo/small/b.go", "go", 30)+`,`+
		duplicateJSON("/repo/large/a.go", "/repo/large/b.go", "go", 300)+`,`+
		duplicateJSON("/repo/mid/a.go", "/repo/mid/b.go", "go", 120)+`]}`)
	want := []float64{300, 120, 30}
	if len(pairs) != len(want) {
		t.Fatalf("got %d pairs, want %d", len(pairs), len(want))
	}
	for index, metric := range want {
		if pairs[index].Metric != metric {
			t.Fatalf("rank %d has metric %v, want %v", index, pairs[index].Metric, metric)
		}
	}
}

func TestReadJSCPDTreatsAbsentReportAsNoFindings(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{
		"unset":   "",
		"missing": filepath.Join(t.TempDir(), "absent.json"),
		"empty":   writeReport(t, ""),
	} {
		pairs, err := ReadJSCPD(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(pairs) != 0 {
			t.Fatalf("%s: got %d pairs, want none", name, len(pairs))
		}
	}
}

func TestJSCPDRefusesEmptyScope(t *testing.T) {
	t.Parallel()

	if _, err := JSCPD(t.Context(), Options{Root: t.TempDir()}); err == nil {
		t.Fatal("a scan with no directories should report an error")
	}
}
