package pipeline

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodestone/internal/analyze"
	"lodestone/internal/report"
)

const (
	alphaOne = "internal/alpha/one.go"
	alphaTwo = "internal/alpha/two.go"
	betaOne  = "internal/beta/one.go"
)

func scanned() []analyze.Function {
	return []analyze.Function{
		{ID: "a", File: alphaOne, Lines: 12},
		{ID: "b", File: alphaOne, Lines: 4},
		{ID: "c", File: alphaTwo, Lines: 9},
		{ID: "d", File: betaOne, Lines: 6},
	}
}

func TestFilterFunctionsKeepsOnlyLongEnough(t *testing.T) {
	t.Parallel()

	kept := filterFunctions(scanned(), 6)
	ids := make([]string, 0, len(kept))
	for _, function := range kept {
		ids = append(ids, function.ID)
	}
	if !slices.Equal(ids, []string{"a", "c", "d"}) {
		t.Fatalf("kept %v", ids)
	}
}

func TestInstructionPrefixAppliesOnlyToTheInstructionAwareProfile(t *testing.T) {
	t.Parallel()

	const task = "Retrieve functions with the same inputs, outputs, and side effects"
	cases := []struct {
		name        string
		profile     string
		instruction string
		want        string
	}{
		{"qwen3 wraps the task", ModelProfileQwen3, task, "Instruct: " + task + "\nQuery:"},
		{"granite declares empty prompts", ModelProfileGranite, task, ""},
		{"an empty task disables the template", ModelProfileQwen3, "", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := instructionPrefix(test.profile, test.instruction); got != test.want {
				t.Errorf("prefix = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFunctionIndexKeysByIdentifier(t *testing.T) {
	t.Parallel()

	index := functionIndex(scanned())
	if len(index) != 4 {
		t.Fatalf("indexed %d functions", len(index))
	}
	if index["c"].File != alphaTwo {
		t.Fatalf("wrong function under key c: %+v", index["c"])
	}
}

func TestDetectorOptionsDeduplicatesFilesAndPrunesDirectories(t *testing.T) {
	t.Parallel()

	options := detectorOptions(Options{Root: "/repo", WorkDir: "/work"}, scanned())
	want := []string{alphaOne, alphaTwo, betaOne}
	if !slices.Equal(options.Files, want) {
		t.Fatalf("files = %v, want %v", options.Files, want)
	}
	if !slices.Equal(options.Directories, []string{"internal/alpha", "internal/beta"}) {
		t.Fatalf("directories = %v", options.Directories)
	}
	if options.Concurrency < 1 {
		t.Fatalf("concurrency = %d", options.Concurrency)
	}
}

func TestDetectorOptionsFallsBackToATemporaryWorkDirectory(t *testing.T) {
	t.Parallel()

	options := detectorOptions(Options{Root: "/repo"}, scanned())
	if options.WorkDir == "" || !filepath.IsAbs(options.WorkDir) {
		t.Fatalf("work directory = %q", options.WorkDir)
	}
}

func TestPrepareDetectorsReportsAMissingBinaryInsteadOfFailing(t *testing.T) {
	t.Parallel()

	var progress bytes.Buffer
	retrieved, detectors := prepareDetectors(
		t.Context(),
		Options{Root: t.TempDir(), WorkDir: t.TempDir()},
		nil,
		&progress,
	)
	if len(detectors) != 2 {
		t.Fatalf("expected one status per conventional retriever, got %d", len(detectors))
	}
	statuses := map[string]string{}
	for _, detector := range detectors {
		statuses[detector.Name] = detector.Status
	}
	if statuses["jscpd"] != report.StatusSkipped {
		t.Fatalf("jscpd status = %q, want skipped when no binary is configured", statuses["jscpd"])
	}
	if statuses["dupl"] != report.StatusRan {
		t.Fatalf("dupl status = %q, want ran because it needs no binary", statuses["dupl"])
	}
	if !strings.Contains(progress.String(), "jscpd skipped") {
		t.Fatalf("a skipped retriever must be announced: %q", progress.String())
	}
	for _, source := range retrieved {
		if source.Name == "jscpd" && len(source.Pairs) != 0 {
			t.Fatal("a skipped retriever contributed pairs")
		}
	}
}

func TestEmbeddingSettingsCarryEveryInferenceKnob(t *testing.T) {
	t.Parallel()

	settings := embeddingSettings(Options{
		Runtime: "/bin/runtime", Model: "/models/granite", ModelProfile: ModelProfileGranite,
		Cache: "/cache.gob",
		Batch: 4, MaxTokens: 512, ChunkTokens: 100, SplitAfter: 200, PauseMS: 50,
	})
	if settings.Runtime != "/bin/runtime" || settings.Model != "/models/granite" {
		t.Fatalf("runtime settings lost: %+v", settings)
	}
	if settings.Profile != ModelProfileGranite {
		t.Fatalf("model profile = %q", settings.Profile)
	}
	if settings.Batch != 4 || settings.MaxTokens != 512 || settings.PauseMS != 50 {
		t.Fatalf("batching settings lost: %+v", settings)
	}
	if settings.ChunkTokens != 100 || settings.SplitAfter != 200 {
		t.Fatalf("windowing settings lost: %+v", settings)
	}
}
