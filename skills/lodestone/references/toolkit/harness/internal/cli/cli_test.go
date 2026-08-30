package cli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodestone/internal/pipeline"
)

func validArguments(extra ...string) []string {
	return append([]string{"--runtime", "/bin/runtime", "--model", "/models/qwen3"}, extra...)
}

func TestParseAcceptsMinimalArguments(t *testing.T) {
	t.Parallel()

	parsed, err := parse(validArguments())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.format != "markdown" {
		t.Fatalf("format = %q", parsed.format)
	}
	if !slices.Equal(parsed.analysis.Patterns, []string{"./..."}) {
		t.Fatalf("patterns = %v", parsed.analysis.Patterns)
	}
	if parsed.analysis.MinScore != defaultMinimumScore || parsed.analysis.Limit != defaultCandidateLimit {
		t.Fatalf("defaults were not applied: %+v", parsed.analysis)
	}
	if parsed.analysis.ModelProfile != pipeline.ModelProfileQwen3 {
		t.Fatalf("model profile = %q", parsed.analysis.ModelProfile)
	}
}

func TestParseUsesModelSpecificScoreUnlessOverridden(t *testing.T) {
	t.Parallel()

	granite, err := parse(validArguments("--model-profile", pipeline.ModelProfileGranite))
	if err != nil {
		t.Fatal(err)
	}
	if granite.analysis.MinScore != pipeline.DefaultMinimumScore(pipeline.ModelProfileGranite) {
		t.Fatalf("granite score = %v", granite.analysis.MinScore)
	}
	explicit, err := parse(validArguments(
		"--model-profile", pipeline.ModelProfileGranite,
		"--min-score", "0.42",
	))
	if err != nil {
		t.Fatal(err)
	}
	if explicit.analysis.MinScore != 0.42 {
		t.Fatalf("explicit score = %v", explicit.analysis.MinScore)
	}
}

func TestParseRejectsInvalidSettings(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		arguments []string
		want      error
	}{
		"no runtime":        {arguments: []string{"--model", "/models/granite"}, want: errMissingRuntime},
		"no model":          {arguments: []string{"--runtime", "/bin/runtime"}, want: errMissingRuntime},
		"zero batch":        {arguments: validArguments("--batch", "0"), want: errPositiveOptions},
		"one max-token":     {arguments: validArguments("--max-tokens", "1"), want: errPositiveOptions},
		"zero limit":        {arguments: validArguments("--limit", "0"), want: errPositiveOptions},
		"pool below limit":  {arguments: validArguments("--limit", "50", "--candidate-pool", "10"), want: errCandidatePool},
		"zero rrf-k":        {arguments: validArguments("--rrf-k", "0"), want: errInvalidRRFK},
		"negative source":   {arguments: validArguments("--source-lines", "-1"), want: errNegativeOptions},
		"negative chunk":    {arguments: validArguments("--chunk-tokens", "-1"), want: errNegativeOptions},
		"score above one":   {arguments: validArguments("--min-score", "1.5"), want: errScoreRange},
		"score below minus": {arguments: validArguments("--min-score", "-2"), want: errScoreRange},
		"empty patterns":    {arguments: validArguments("--patterns", " , "), want: errEmptyPatterns},
		"unknown format":    {arguments: validArguments("--format", "yaml"), want: errInvalidFormat},
		"unknown model":     {arguments: validArguments("--model-profile", "other"), want: errInvalidModel},
	} {
		_, err := parse(testCase.arguments)
		if !errors.Is(err, testCase.want) {
			t.Fatalf("%s: got %v, want %v", name, err, testCase.want)
		}
	}
}

func TestParseRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	if _, err := parse(validArguments("--nope")); err == nil {
		t.Fatal("an unknown flag should fail rather than be ignored")
	}
}

func TestParseResolvesPathsAgainstTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	parsed, err := parse(validArguments("--root", "relative/repo", "--output", "out/report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"root":   parsed.analysis.Root,
		"output": parsed.output,
	} {
		if !filepath.IsAbs(value) {
			t.Fatalf("%s was not resolved to an absolute path: %q", name, value)
		}
	}
	if !strings.HasSuffix(parsed.analysis.Root, "relative/repo") {
		t.Fatalf("root lost its suffix: %q", parsed.analysis.Root)
	}
}

func TestParseLeavesUnsetPathsEmpty(t *testing.T) {
	t.Parallel()

	parsed, err := parse(validArguments())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"output":       parsed.output,
		"cache":        parsed.analysis.Cache,
		"jscpd report": parsed.analysis.JSCPDReport,
		"jscpd binary": parsed.analysis.JSCPDBinary,
		"work dir":     parsed.analysis.WorkDir,
	} {
		if value != "" {
			t.Fatalf("%s was filled in from an empty flag: %q", name, value)
		}
	}
}

func TestSplitPatternsTrimsAndDropsEmpty(t *testing.T) {
	t.Parallel()

	got := splitPatterns(" ./internal/alpha , ,./internal/beta ")
	want := []string{"./internal/alpha", "./internal/beta"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
