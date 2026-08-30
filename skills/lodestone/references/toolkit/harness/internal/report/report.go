package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"lodestone/internal/analyze"
	"lodestone/internal/fusion"
)

var errFormat = errors.New("unsupported report format")

const (
	StatusRan      = "ran"
	StatusProvided = "provided"
	StatusSkipped  = "skipped"
	StatusFailed   = "failed"
)

type Detector struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Pairs  int    `json:"pairs"`
	Detail string `json:"detail,omitempty"`
}

type Result struct {
	Root                  string             `json:"root"`
	Patterns              []string           `json:"patterns"`
	ModelProfile          string             `json:"modelProfile"`
	FunctionCount         int                `json:"functionCount"`
	CandidateCount        int                `json:"candidateCount"`
	MinimumSemanticScore  float64            `json:"minimumSemanticScore"`
	Representation        string             `json:"representation"`
	EmbeddingMilliseconds float64            `json:"embeddingMilliseconds"`
	RRFK                  float64            `json:"rrfK"`
	Detectors             []Detector         `json:"detectors"`
	GeneratedFiles        []string           `json:"generatedFiles,omitempty"`
	IgnoredFunctions      []string           `json:"ignoredFunctions,omitempty"`
	Candidates            []fusion.Candidate `json:"candidates"`
}

func AttachEvidence(
	candidates []fusion.Candidate,
	functions map[string]analyze.Function,
	sourceLines int,
) {
	for index := range candidates {
		left := functions[candidates[index].Left.ID]
		right := functions[candidates[index].Right.ID]
		candidates[index].SequenceScore = analyze.SequenceSimilarity(left, right)
		if sourceLines <= 0 {
			continue
		}
		candidates[index].LeftSource = clip(left.Source, sourceLines)
		candidates[index].RightSource = clip(right.Source, sourceLines)
	}
}

func clip(source string, maxLines int) string {
	if source == "" {
		return ""
	}
	lines := strings.Split(source, "\n")
	if len(lines) <= maxLines {
		return source
	}

	return strings.Join(lines[:maxLines], "\n") +
		fmt.Sprintf("\n// ... %d more lines", len(lines)-maxLines)
}

func Write(output io.Writer, format string, result Result) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fmt.Errorf("encode JSON report: %w", err)
		}

		return nil
	case "markdown":
		return writeMarkdown(output, result)
	default:
		return fmt.Errorf("%w: %q", errFormat, format)
	}
}

func writeMarkdown(output io.Writer, result Result) error {
	if err := writeHeader(output, result); err != nil {
		return err
	}
	if len(result.Candidates) == 0 {
		return writeText(output, "No candidates met the configured score.\n")
	}
	if err := writeTable(output, result.Candidates); err != nil {
		return err
	}

	return writeDetails(output, result.Candidates)
}

func writeHeader(output io.Writer, result Result) error {
	const header = "# Hybrid code-similarity candidates\n\n" +
		"- Repository: `%s`\n" +
		"- Patterns: `%s`\n" +
		"- Functions: %d\n" +
		"- Candidates: %d\n" +
		"- Embedding model: `%s`\n" +
		"- Representation: `%s`\n" +
		"- RRF k: %.0f\n" +
		"- Minimum semantic score: %.4f\n" +
		"- Model inference: %.0fms\n\n"
	if err := writeText(
		output, header,
		result.Root,
		strings.Join(result.Patterns, ", "),
		result.FunctionCount,
		result.CandidateCount,
		result.ModelProfile,
		result.Representation,
		result.RRFK,
		result.MinimumSemanticScore,
		result.EmbeddingMilliseconds,
	); err != nil {
		return err
	}

	if err := writeDetectors(output, result.Detectors); err != nil {
		return err
	}
	if err := writeExclusions(output, result.GeneratedFiles, result.IgnoredFunctions); err != nil {
		return err
	}

	return writeText(
		output,
		"RRF combines candidate ranks from semantic, jscpd, and dupl retrieval. "+
			"The result does not establish that two functions are duplicates.\n\n",
	)
}

func writeDetectors(output io.Writer, detectors []Detector) error {
	if len(detectors) == 0 {
		return nil
	}
	const heading = "## Retrievers\n\n| Source | Status | Pairs | Detail |\n|---|---|---:|---|\n"
	if err := writeText(output, heading); err != nil {
		return err
	}
	for _, detector := range detectors {
		if err := writeText(
			output, "| `%s` | %s | %d | %s |\n",
			detector.Name, detector.Status, detector.Pairs, cell(detector.Detail),
		); err != nil {
			return err
		}
	}
	if missing(detectors) {
		if err := writeText(
			output,
			"\n> A `skipped` or `failed` retriever contributed no ranks. "+
				"Read the fused order as covering only the sources marked `ran` or `provided`.\n",
		); err != nil {
			return err
		}
	}

	return writeText(output, "\n")
}

func writeExclusions(output io.Writer, generated, ignored []string) error {
	if len(generated) > 0 {
		if err := writeText(
			output,
			"Skipped %d generated files: %s. Pass `--include-generated` to keep them.\n\n",
			len(generated), summarize(generated),
		); err != nil {
			return err
		}
	}
	if len(ignored) == 0 {
		return nil
	}

	return writeText(
		output,
		"Skipped %d functions marked `%s`: %s.\n\n",
		len(ignored), analyze.IgnoreMarker, summarize(ignored),
	)
}

func summarize(names []string) string {
	const listed = 10
	if len(names) <= listed {
		return list(names)
	}

	return list(names[:listed]) + fmt.Sprintf(", and %d more", len(names)-listed)
}

func cell(detail string) string {
	if detail == "" {
		return "-"
	}
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "|", "\\|")

	return replacer.Replace(detail)
}

func missing(detectors []Detector) bool {
	for _, detector := range detectors {
		if detector.Status == StatusSkipped || detector.Status == StatusFailed {
			return true
		}
	}

	return false
}

func writeTable(output io.Writer, candidates []fusion.Candidate) error {
	const heading = "| # | RRF | Sources | Semantic | Separation | Structure | Sequence |" +
		" Size | Calls | Locality | Functions |\n" +
		"|---:|---:|---|---:|---:|---:|---:|---:|---:|---|---|\n"
	if err := writeText(output, heading); err != nil {
		return err
	}
	for index, candidate := range candidates {
		if err := writeText(
			output,
			"| %d | %.6f | %s | %s | %.3f | %.4f | %.4f | %.4f | %.4f | %s | `%s:%d` `%s` ↔ `%s:%d` `%s` |\n",
			index+1,
			candidate.RRFScore,
			sources(candidate.Sources),
			semantic(candidate),
			candidate.Separation,
			candidate.StructureScore,
			candidate.SequenceScore,
			candidate.SizeRatio,
			candidate.CallScore,
			candidate.Locality,
			candidate.Left.File,
			candidate.Left.Line,
			candidate.Left.Name,
			candidate.Right.File,
			candidate.Right.Line,
			candidate.Right.Name,
		); err != nil {
			return err
		}
	}

	return writeText(output, "\n")
}

func writeDetails(output io.Writer, candidates []fusion.Candidate) error {
	const details = "## %d. `%s` and `%s`\n\n" +
		"- Left: `%s:%d`\n" +
		"- Right: `%s:%d`\n" +
		"- RRF: %.6f\n" +
		"- Sources: %s\n" +
		"- Semantic: %s\n" +
		"- Separation: %.3f\n" +
		"- Structure (node mix): %.4f\n" +
		"- Sequence (statement order): %.4f\n" +
		"- Size ratio (statement counts): %.4f\n" +
		"- Call overlap: %.4f\n" +
		"- Locality: %s\n" +
		"- Shared calls: %s\n" +
		"- Left-only calls: %s\n" +
		"- Right-only calls: %s\n\n"
	for index, candidate := range candidates {
		if err := writeText(
			output,
			details,
			index+1,
			candidate.Left.Name,
			candidate.Right.Name,
			candidate.Left.File,
			candidate.Left.Line,
			candidate.Right.File,
			candidate.Right.Line,
			candidate.RRFScore,
			sources(candidate.Sources),
			semantic(candidate),
			candidate.Separation,
			candidate.StructureScore,
			candidate.SequenceScore,
			candidate.SizeRatio,
			candidate.CallScore,
			candidate.Locality,
			list(candidate.SharedCalls),
			list(candidate.LeftOnlyCalls),
			list(candidate.RightOnlyCalls),
		); err != nil {
			return err
		}
		if err := writeSources(output, candidate); err != nil {
			return err
		}
	}

	return nil
}

func writeSources(output io.Writer, candidate fusion.Candidate) error {
	pairs := []struct {
		label    string
		function analyze.FunctionRef
		source   string
	}{
		{label: "left", function: candidate.Left, source: candidate.LeftSource},
		{label: "right", function: candidate.Right, source: candidate.RightSource},
	}
	for _, pair := range pairs {
		if pair.source == "" {
			continue
		}
		fence := fenceFor(pair.source)
		if err := writeText(
			output,
			"<details><summary>%s: <code>%s:%d</code> <code>%s</code></summary>\n\n%sgo\n%s\n%s\n\n</details>\n\n",
			pair.label, pair.function.File, pair.function.Line, pair.function.Name, fence, pair.source, fence,
		); err != nil {
			return err
		}
	}

	return nil
}

func writeText(output io.Writer, format string, values ...any) error {
	if _, err := fmt.Fprintf(output, format, values...); err != nil {
		return fmt.Errorf("write Markdown report: %w", err)
	}

	return nil
}

func list(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return "`" + strings.Join(values, "`, `") + "`"
}

func sources(values []fusion.Source) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("`%s:%d`", value.Name, value.Rank))
	}
	return strings.Join(parts, ", ")
}

// semantic renders the cosine only for pairs the semantic retriever actually
// scored; a detector-only pair never cleared the threshold and a zero there
// would read as a measured dissimilarity.
func semantic(candidate fusion.Candidate) string {
	for _, source := range candidate.Sources {
		if source.Name == "semantic" {
			return fmt.Sprintf("%.4f", candidate.SemanticScore)
		}
	}

	return "-"
}

// fenceFor returns a code fence longer than any backtick run in the source,
// so a raw-string literal containing a fence cannot break the block.
func fenceFor(source string) string {
	longest := 0
	current := 0
	for _, character := range source {
		if character == '`' {
			current++
			longest = max(longest, current)

			continue
		}
		current = 0
	}

	const minimumFence = 3

	return strings.Repeat("`", max(minimumFence, longest+1))
}
