package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"lodestone/internal/pipeline"
	"lodestone/internal/report"
)

const (
	defaultBatchSize      = 8
	defaultCandidateLimit = 50
	defaultCandidatePool  = 300
	defaultMaximumTokens  = 768
	defaultMinimumLines   = 6
	defaultMinimumNodes   = 3
	defaultMinimumTokens  = 50
	defaultDuplThreshold  = 150
	defaultSourceLines    = 80
	defaultMinimumScore   = 0.65
	defaultPauseMS        = 100
	defaultRRFK           = 60
	directoryMode         = 0o750
	maximumScore          = 1
	minimumScore          = -1
)

var (
	errCandidatePool   = errors.New("candidate-pool must be at least limit")
	errEmptyPatterns   = errors.New("patterns must not be empty")
	errInvalidFormat   = errors.New("format must be markdown or json")
	errInvalidModel    = errors.New("model-profile must be qwen3 or granite")
	errInvalidRRFK     = errors.New("rrf-k must be positive")
	errMissingRuntime  = errors.New("--runtime and --model are required")
	errNegativeOptions = errors.New(
		"chunk-tokens, split-after, pause-ms, source-lines, and min-tokens must not be negative",
	)
	errPositiveOptions = errors.New("batch, min-lines, and limit must be positive, and max-tokens at least 2")
	errScoreRange      = errors.New("min-score must be between -1 and 1")
)

type settings struct {
	analysis pipeline.Options
	output   string
	format   string
}

func Run(ctx context.Context, arguments []string, stdout, stderr io.Writer) error {
	parsed, err := parse(arguments)
	if err != nil {
		return err
	}
	result, err := pipeline.Run(ctx, parsed.analysis, stderr)
	if err != nil {
		return fmt.Errorf("analyze repository: %w", err)
	}

	return writeResult(stdout, stderr, parsed, result)
}

func parse(arguments []string) (settings, error) {
	flags := flag.NewFlagSet("code-similarity", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var parsed settings
	var patterns string
	bind(flags, &parsed, &patterns)
	if err := flags.Parse(arguments); err != nil {
		return settings{}, fmt.Errorf("parse options: %w", err)
	}
	minimumScoreSet := false
	flags.Visit(func(current *flag.Flag) {
		minimumScoreSet = minimumScoreSet || current.Name == "min-score"
	})
	if !minimumScoreSet {
		parsed.analysis.MinScore = pipeline.DefaultMinimumScore(parsed.analysis.ModelProfile)
	}
	parsed.analysis.Patterns = splitPatterns(patterns)
	if err := validate(parsed); err != nil {
		return settings{}, err
	}

	return resolve(parsed)
}

func bind(flags *flag.FlagSet, parsed *settings, patterns *string) {
	analysis := &parsed.analysis
	flags.StringVar(&analysis.Root, "root", ".", "repository root")
	flags.StringVar(&analysis.Runtime, "runtime", "", "MLX embedding runtime")
	flags.StringVar(&analysis.Model, "model", "", "MLX embedding model")
	flags.StringVar(
		&analysis.ModelProfile,
		"model-profile",
		pipeline.ModelProfileQwen3,
		"embedding profile: qwen3 or granite",
	)
	flags.StringVar(patterns, "patterns", "./...", "comma-separated go list patterns")
	flags.StringVar(&parsed.output, "output", "", "report path")
	flags.StringVar(&analysis.Cache, "cache", "", "persistent embedding cache")
	flags.StringVar(&analysis.Instruction, "instruction", "",
		"retrieval task applied to every unit; empty disables the model instruction template")
	flags.StringVar(&analysis.JSCPDReport, "jscpd-report", "", "jscpd JSON report")
	flags.StringVar(&analysis.JSCPDBinary, "jscpd", "",
		"jscpd binary; runs the detector when no report is supplied")
	flags.StringVar(&analysis.WorkDir, "work-dir", "", "directory for generated detector reports")
	flags.StringVar(&parsed.format, "format", "markdown", "markdown or json")
	flags.IntVar(&analysis.Batch, "batch", defaultBatchSize, "embedding batch size")
	flags.IntVar(&analysis.MaxTokens, "max-tokens", defaultMaximumTokens, "maximum tokens per function")
	flags.IntVar(&analysis.MinLines, "min-lines", defaultMinimumLines, "minimum function line count")
	flags.IntVar(&analysis.MinNodes, "min-nodes", defaultMinimumNodes,
		"minimum statements per function; line count alone lets a one-statement function through")
	flags.IntVar(&analysis.MinTokens, "min-tokens", defaultMinimumTokens, "minimum jscpd clone token count")
	flags.IntVar(&analysis.DuplThreshold, "dupl-threshold", defaultDuplThreshold,
		"minimum dupl clone token count")
	flags.IntVar(&analysis.SourceLines, "source-lines", defaultSourceLines,
		"function lines carried into the report; zero omits source")
	flags.Float64Var(&analysis.MinScore, "min-score", defaultMinimumScore, "minimum semantic score")
	flags.IntVar(&analysis.Limit, "limit", defaultCandidateLimit, "maximum report candidates")
	flags.IntVar(&analysis.Pool, "candidate-pool", defaultCandidatePool,
		"semantic candidates retained before fusion")
	flags.Float64Var(&analysis.RRFK, "rrf-k", defaultRRFK, "reciprocal rank fusion constant")
	flags.IntVar(&analysis.ChunkTokens, "chunk-tokens", 0,
		"overlapping token window size; zero embeds whole functions")
	flags.IntVar(&analysis.SplitAfter, "split-after", 0, "only split functions above this token count")
	flags.IntVar(&analysis.PauseMS, "pause-ms", defaultPauseMS, "pause between uncached inference batches")
	flags.BoolVar(&analysis.IncludeTests, "include-tests", false, "include test functions")
	flags.BoolVar(&analysis.IncludeGenerated, "include-generated", false,
		"include machine-generated files, which are duplicated by construction")
}

func validate(parsed settings) error {
	checks := []struct {
		invalid bool
		err     error
	}{
		{invalid: parsed.analysis.Runtime == "" || parsed.analysis.Model == "", err: errMissingRuntime},
		{invalid: !pipeline.ValidModelProfile(parsed.analysis.ModelProfile), err: errInvalidModel},
		{invalid: hasNonPositive(parsed.analysis), err: errPositiveOptions},
		{invalid: parsed.analysis.Pool < parsed.analysis.Limit, err: errCandidatePool},
		{invalid: parsed.analysis.RRFK <= 0, err: errInvalidRRFK},
		{invalid: hasNegative(parsed.analysis), err: errNegativeOptions},
		{
			invalid: parsed.analysis.MinScore < minimumScore || parsed.analysis.MinScore > maximumScore,
			err:     errScoreRange,
		},
		{invalid: len(parsed.analysis.Patterns) == 0, err: errEmptyPatterns},
		{invalid: parsed.format != "markdown" && parsed.format != "json", err: errInvalidFormat},
	}
	for _, check := range checks {
		if check.invalid {
			return check.err
		}
	}

	return nil
}

func hasNonPositive(analysis pipeline.Options) bool {
	return analysis.Batch < 1 || analysis.MaxTokens < 2 ||
		analysis.MinLines < 1 || analysis.Limit < 1
}

func hasNegative(analysis pipeline.Options) bool {
	return analysis.ChunkTokens < 0 || analysis.SplitAfter < 0 || analysis.PauseMS < 0 ||
		analysis.SourceLines < 0 || analysis.MinTokens < 0 || analysis.DuplThreshold < 0 ||
		analysis.MinNodes < 0
}

func resolve(parsed settings) (settings, error) {
	paths := []struct {
		name  string
		value *string
	}{
		{name: "repository root", value: &parsed.analysis.Root},
		{name: "runtime path", value: &parsed.analysis.Runtime},
		{name: "model path", value: &parsed.analysis.Model},
		{name: "report path", value: &parsed.output},
		{name: "cache path", value: &parsed.analysis.Cache},
		{name: "jscpd report", value: &parsed.analysis.JSCPDReport},
		{name: "jscpd binary", value: &parsed.analysis.JSCPDBinary},
		{name: "work directory", value: &parsed.analysis.WorkDir},
	}
	for _, path := range paths {
		if *path.value == "" {
			continue
		}
		absolute, err := filepath.Abs(*path.value)
		if err != nil {
			return settings{}, fmt.Errorf("resolve %s: %w", path.name, err)
		}
		*path.value = absolute
	}
	// jscpd reports absolute paths from the detector's real working
	// directory, so a symlinked root (such as /tmp on macOS) must be
	// resolved or every reported clone fails to map back onto a function.
	if resolved, err := filepath.EvalSymlinks(parsed.analysis.Root); err == nil {
		parsed.analysis.Root = resolved
	}

	return parsed, nil
}

func writeResult(stdout, stderr io.Writer, parsed settings, result report.Result) error {
	if parsed.output == "" {
		if err := report.Write(stdout, parsed.format, result); err != nil {
			return fmt.Errorf("write report: %w", err)
		}

		return nil
	}
	if err := os.MkdirAll(filepath.Dir(parsed.output), directoryMode); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	file, err := os.Create(parsed.output)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	writeErr := report.Write(file, parsed.format, result)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write report: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close report: %w", closeErr)
	}
	if _, err := fmt.Fprintf(
		stderr, "wrote %d candidates to %s\n", len(result.Candidates), parsed.output,
	); err != nil {
		return fmt.Errorf("write report progress: %w", err)
	}

	return nil
}

func splitPatterns(value string) []string {
	var patterns []string
	for pattern := range strings.SplitSeq(value, ",") {
		if pattern = strings.TrimSpace(pattern); pattern != "" {
			patterns = append(patterns, pattern)
		}
	}

	return patterns
}
