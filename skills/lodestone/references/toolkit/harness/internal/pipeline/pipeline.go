package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"lodestone/internal/analyze"
	"lodestone/internal/detect"
	"lodestone/internal/embed"
	"lodestone/internal/fusion"
	"lodestone/internal/report"
)

const minimumFunctionCount = 2

const (
	ModelProfileQwen3   = "qwen3"
	ModelProfileGranite = "granite"

	minimumScoreQwen3   = 0.65
	minimumScoreGranite = 0.80
)

var ErrTooFewFunctions = errors.New("need at least two functions after filtering")

type Options struct {
	Root             string
	Runtime          string
	Model            string
	ModelProfile     string
	Instruction      string
	Cache            string
	JSCPDReport      string
	JSCPDBinary      string
	WorkDir          string
	Patterns         []string
	Batch            int
	MaxTokens        int
	MinLines         int
	MinNodes         int
	MinTokens        int
	DuplThreshold    int
	SourceLines      int
	Limit            int
	Pool             int
	ChunkTokens      int
	SplitAfter       int
	PauseMS          int
	MinScore         float64
	RRFK             float64
	IncludeTests     bool
	IncludeGenerated bool
}

type detectorRun struct {
	name   string
	status string
	detail string
	pairs  []detect.Pair
	err    error
}

func Run(ctx context.Context, settings Options, progress io.Writer) (report.Result, error) {
	scanned, err := analyze.Scan(ctx, analyze.ScanOptions{
		Root:             settings.Root,
		Patterns:         settings.Patterns,
		MinLines:         1,
		MinNodes:         settings.MinNodes,
		IncludeTests:     settings.IncludeTests,
		IncludeGenerated: settings.IncludeGenerated,
	})
	if err != nil {
		return report.Result{}, fmt.Errorf("scan Go functions: %w", err)
	}
	functions := filterFunctions(scanned.Functions, settings.MinLines)
	if len(functions) < minimumFunctionCount {
		return report.Result{}, ErrTooFewFunctions
	}
	if _, err := fmt.Fprintf(
		progress, "scanned %d functions, skipped %d generated files and %d ignored functions\n",
		len(functions), len(scanned.Generated), len(scanned.Ignored),
	); err != nil {
		return report.Result{}, fmt.Errorf("write scan progress: %w", err)
	}
	retrieved, detectors := prepareDetectors(ctx, settings, scanned.Functions, progress)
	vectors, err := embed.Functions(ctx, functions, embeddingSettings(settings), progress)
	if err != nil {
		return report.Result{}, fmt.Errorf("embed functions: %w", err)
	}
	fused, err := rankAndFuse(settings, scanned.Functions, functions, vectors, retrieved)
	if err != nil {
		return report.Result{}, err
	}

	return makeResult(settings, scanned, functions, vectors, fused, detectors), nil
}

func rankAndFuse(
	settings Options,
	all, functions []analyze.Function,
	vectors embed.Vectors,
	retrieved []fusion.Retrieved,
) (fusion.Result, error) {
	semantic, err := analyze.Rank(
		functions, vectors.Embeddings, settings.MinScore, settings.Pool,
	)
	if err != nil {
		return fusion.Result{}, fmt.Errorf("rank semantic candidates: %w", err)
	}
	fused, err := fusion.Fuse(
		settings.Root, all, semantic, retrieved,
		settings.RRFK, settings.Limit, settings.IncludeTests,
	)
	if err != nil {
		return fusion.Result{}, fmt.Errorf("fuse candidate ranks: %w", err)
	}

	return fused, nil
}

func makeResult(
	settings Options,
	scanned analyze.ScanResult,
	functions []analyze.Function,
	vectors embed.Vectors,
	fused fusion.Result,
	detectors []report.Detector,
) report.Result {
	detectors = append(
		[]report.Detector{{Name: "semantic", Status: report.StatusRan}},
		detectors...,
	)
	for index := range detectors {
		detectors[index].Pairs = fused.Counts[detectors[index].Name]
	}
	report.AttachEvidence(fused.Candidates, functionIndex(scanned.Functions), settings.SourceLines)

	return report.Result{
		Root:                  settings.Root,
		Patterns:              settings.Patterns,
		ModelProfile:          settings.ModelProfile,
		FunctionCount:         len(functions),
		CandidateCount:        len(fused.Candidates),
		MinimumSemanticScore:  settings.MinScore,
		Representation:        embed.Representation(embeddingSettings(settings)),
		RRFK:                  settings.RRFK,
		EmbeddingMilliseconds: float64(vectors.Time) / float64(time.Millisecond),
		Detectors:             detectors,
		GeneratedFiles:        scanned.Generated,
		IgnoredFunctions:      scanned.Ignored,
		Candidates:            fused.Candidates,
	}
}

func embeddingSettings(settings Options) embed.Settings {
	return embed.Settings{
		Runtime:     settings.Runtime,
		Model:       settings.Model,
		Profile:     settings.ModelProfile,
		Instruction: instructionPrefix(settings.ModelProfile, settings.Instruction),
		Cache:       settings.Cache,
		Batch:       settings.Batch,
		MaxTokens:   settings.MaxTokens,
		ChunkTokens: settings.ChunkTokens,
		SplitAfter:  settings.SplitAfter,
		PauseMS:     settings.PauseMS,
	}
}

// instructionPrefix applies the selected model's instruction template to every
// unit, so both sides of a pair are encoded the same way. Granite's Sentence
// Transformers configuration declares empty prompts, so it receives none.
func instructionPrefix(profile, instruction string) string {
	if instruction == "" || profile != ModelProfileQwen3 {
		return ""
	}

	return "Instruct: " + instruction + "\nQuery:"
}

func ValidModelProfile(profile string) bool {
	return profile == ModelProfileQwen3 || profile == ModelProfileGranite
}

func DefaultMinimumScore(profile string) float64 {
	if profile == ModelProfileGranite {
		return minimumScoreGranite
	}

	return minimumScoreQwen3
}

func prepareDetectors(
	ctx context.Context,
	settings Options,
	functions []analyze.Function,
	progress io.Writer,
) ([]fusion.Retrieved, []report.Detector) {
	shared := detectorOptions(settings, functions)
	runs := []detectorRun{runJSCPD(ctx, settings, shared), runDupl(ctx, shared)}
	retrieved := make([]fusion.Retrieved, 0, len(runs))
	detectors := make([]report.Detector, 0, len(runs))
	for _, item := range runs {
		detector := report.Detector{Name: item.name, Status: item.status}
		if item.err != nil {
			detector.Status = report.StatusFailed
			detector.Detail = item.err.Error()
		} else {
			detector.Detail = item.detail
			retrieved = append(retrieved, fusion.Retrieved{Name: item.name, Pairs: item.pairs})
		}
		detectors = append(detectors, detector)
		if detector.Status == report.StatusRan || detector.Status == report.StatusProvided {
			continue
		}
		_, _ = fmt.Fprintf(
			progress, "retriever %s %s: %s\n", detector.Name, detector.Status, detector.Detail,
		)
	}

	return retrieved, detectors
}

func detectorOptions(settings Options, functions []analyze.Function) detect.Options {
	seen := make(map[string]struct{}, len(functions))
	files := make([]string, 0, len(functions))
	for _, function := range functions {
		if _, ok := seen[function.File]; ok {
			continue
		}
		seen[function.File] = struct{}{}
		files = append(files, function.File)
	}
	workDir := settings.WorkDir
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "code-similarity-detectors")
	}

	return detect.Options{
		Root:          settings.Root,
		WorkDir:       workDir,
		Directories:   detect.Directories(files),
		Files:         files,
		MinLines:      settings.MinLines,
		MinTokens:     settings.MinTokens,
		DuplThreshold: settings.DuplThreshold,
		Concurrency:   runtime.GOMAXPROCS(0),
	}
}

func runJSCPD(ctx context.Context, settings Options, shared detect.Options) detectorRun {
	const name = "jscpd"
	if settings.JSCPDReport != "" {
		if _, err := os.Stat(settings.JSCPDReport); err != nil {
			return detectorRun{name: name, status: report.StatusFailed, detail: err.Error()}
		}
		pairs, err := detect.ReadJSCPD(settings.JSCPDReport)

		return detectorRun{name: name, status: report.StatusProvided, pairs: pairs, err: err}
	}
	if !detect.Available(settings.JSCPDBinary) {
		return detectorRun{
			name:   name,
			status: report.StatusSkipped,
			detail: "no executable at " + settings.JSCPDBinary,
		}
	}
	shared.Binary = settings.JSCPDBinary
	pairs, err := detect.JSCPD(ctx, shared)

	return detectorRun{name: name, status: report.StatusRan, pairs: pairs, err: err}
}

func runDupl(ctx context.Context, shared detect.Options) detectorRun {
	pairs, err := detect.Dupl(ctx, shared)

	return detectorRun{name: "dupl", status: report.StatusRan, pairs: pairs, err: err}
}

func functionIndex(functions []analyze.Function) map[string]analyze.Function {
	index := make(map[string]analyze.Function, len(functions))
	for _, function := range functions {
		index[function.ID] = function
	}

	return index
}

func filterFunctions(functions []analyze.Function, minLines int) []analyze.Function {
	result := make([]analyze.Function, 0, len(functions))
	for _, function := range functions {
		if function.Lines >= minLines {
			result = append(result, function)
		}
	}

	return result
}
