package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/golangci/dupl/lib"
)

const (
	directoryMode = 0o750

	jscpdThreshold     = "100"
	jscpdOutputName    = "jscpd-report.json"
	maximumExitCode    = 1
	jscpdArgumentCount = 18
)

var (
	errNoJSCPDReport = errors.New("jscpd wrote no report")
	errNoDirectories = errors.New("no scanned directories to search")
)

type Fragment struct {
	File  string
	Start int
	End   int
}

type Pair struct {
	First  Fragment
	Second Fragment
	Metric float64
}

type Options struct {
	Root          string
	WorkDir       string
	Binary        string
	Directories   []string
	Files         []string
	MinLines      int
	MinTokens     int
	DuplThreshold int
	Concurrency   int
}

func Available(binary string) bool {
	if binary == "" {
		return false
	}
	info, err := os.Stat(binary)
	if err != nil {
		return false
	}

	return !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

func JSCPD(ctx context.Context, settings Options) ([]Pair, error) {
	if len(settings.Directories) == 0 {
		return nil, errNoDirectories
	}
	directory := filepath.Join(settings.WorkDir, "jscpd")
	report := filepath.Join(directory, jscpdOutputName)
	// A leftover report from an earlier run must not masquerade as this
	// run's findings when the detector fails before writing a fresh one.
	if err := os.Remove(report); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear stale jscpd report: %w", err)
	}
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return nil, fmt.Errorf("create jscpd work directory: %w", err)
	}
	tolerated, err := runDetector(ctx, settings.Binary, jscpdArguments(directory, settings), settings.Root)
	if err != nil {
		return nil, err
	}
	// Exit code 1 is advisory when a report exists; without one it was a
	// usage or input error that would otherwise read as a clean run.
	if _, statErr := os.Stat(report); tolerated != "" && errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", errNoJSCPDReport, truncate(tolerated))
	}

	return ReadJSCPD(report)
}

func jscpdArguments(directory string, settings Options) []string {
	arguments := make([]string, 0, jscpdArgumentCount+len(settings.Directories))
	arguments = append(arguments,
		"--no-colors",
		"--absolute",
		"--threshold", jscpdThreshold,
		"--mode", "weak",
		"--format", "go",
		"--min-lines", strconv.Itoa(settings.MinLines),
		"--min-tokens", strconv.Itoa(settings.MinTokens),
		"--workers", strconv.Itoa(settings.Concurrency),
		"--reporters", "json",
		"--output", directory,
	)

	return append(arguments, settings.Directories...)
}

func Dupl(_ context.Context, settings Options) ([]Pair, error) {
	if len(settings.Files) == 0 {
		return nil, nil
	}
	files := make([]string, 0, len(settings.Files))
	for _, file := range settings.Files {
		files = append(files, filepath.Join(settings.Root, file))
	}
	issues, err := lib.Run(files, settings.DuplThreshold)
	if err != nil {
		return nil, fmt.Errorf("run dupl: %w", err)
	}
	pairs := make([]Pair, 0, len(issues))
	seen := make(map[string]bool, len(issues))
	for _, issue := range issues {
		first := Fragment{
			File: issue.From.Filename(), Start: issue.From.LineStart(), End: issue.From.LineEnd(),
		}
		second := Fragment{
			File: issue.To.Filename(), Start: issue.To.LineStart(), End: issue.To.LineEnd(),
		}
		// lib.Run links a clone group as a ring, so a two-clone group
		// arrives in both directions; keep one canonical orientation.
		if fragmentLess(second, first) {
			first, second = second, first
		}
		key := fragmentKey(first) + "|" + fragmentKey(second)
		if seen[key] {
			continue
		}
		seen[key] = true
		pairs = append(pairs, Pair{
			First: first, Second: second, Metric: float64(min(span(first), span(second))),
		})
	}

	return rank(pairs), nil
}

func ReadJSCPD(path string) ([]Pair, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat jscpd report: %w", err)
	}
	if info.Size() == 0 {
		return nil, nil
	}
	var document jscpdReport
	if err := decodeFile(path, &document); err != nil {
		return nil, err
	}
	pairs := make([]Pair, 0, len(document.Duplicates))
	for _, duplicate := range document.Duplicates {
		if duplicate.Format != "go" {
			continue
		}
		pairs = append(pairs, Pair{
			First:  Fragment{File: duplicate.First.Name, Start: duplicate.First.Start, End: duplicate.First.End},
			Second: Fragment{File: duplicate.Second.Name, Start: duplicate.Second.Start, End: duplicate.Second.End},
			Metric: float64(duplicate.Tokens),
		})
	}

	return rank(pairs), nil
}

func Directories(files []string) []string {
	unique := make(map[string]struct{})
	for _, file := range files {
		directory := filepath.ToSlash(filepath.Dir(file))
		if directory == "" {
			directory = "."
		}
		unique[directory] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for directory := range unique {
		result = append(result, directory)
	}
	slices.Sort(result)

	return prune(result)
}

type jscpdReport struct {
	Duplicates []jscpdDuplicate `json:"duplicates"`
}

type jscpdFragment struct {
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type jscpdDuplicate struct {
	First  jscpdFragment `json:"firstFile"`
	Second jscpdFragment `json:"secondFile"`
	Format string        `json:"format"`
	Lines  int           `json:"lines"`
	Tokens int           `json:"tokens"`
}

func rank(pairs []Pair) []Pair {
	slices.SortStableFunc(pairs, func(left, right Pair) int {
		if left.Metric != right.Metric {
			if left.Metric > right.Metric {
				return -1
			}

			return 1
		}

		return strings.Compare(key(left), key(right))
	})

	return pairs
}

func key(pair Pair) string {
	return pair.First.File + ":" + strconv.Itoa(pair.First.Start) + "|" +
		pair.Second.File + ":" + strconv.Itoa(pair.Second.Start)
}

func span(fragment Fragment) int {
	return fragment.End - fragment.Start + 1
}

func prune(sorted []string) []string {
	result := make([]string, 0, len(sorted))
	for _, directory := range sorted {
		if len(result) > 0 && covers(result[len(result)-1], directory) {
			continue
		}
		result = append(result, directory)
	}

	return result
}

func covers(parent, child string) bool {
	if parent == child || parent == "." {
		return true
	}

	return strings.HasPrefix(child, parent+"/")
}

// runDetector runs the detector and returns its stderr when it exited with a
// tolerated advisory code, so the caller can tell findings from failures.
func runDetector(ctx context.Context, binary string, arguments []string, directory string) (string, error) {
	command := exec.CommandContext(ctx, binary, arguments...) //nolint:gosec // detector path comes from a trusted flag
	command.Dir = directory
	var messages bytes.Buffer
	command.Stdout = io.Discard
	command.Stderr = &messages
	err := command.Run()
	if err == nil {
		return "", nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() <= maximumExitCode {
		tolerated := strings.TrimSpace(messages.String())
		if tolerated == "" {
			tolerated = "exit code 1 with no error output"
		}

		return tolerated, nil
	}

	return "", fmt.Errorf("run %s: %w: %s", filepath.Base(binary), err, truncate(messages.String()))
}

func decodeFile(path string, value any) error {
	directory, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open report directory: %w", err)
	}
	defer func() { _ = directory.Close() }()
	file, err := directory.Open(filepath.Base(path))
	if err != nil {
		return fmt.Errorf("open report: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := json.NewDecoder(file).Decode(value); err != nil {
		return fmt.Errorf("decode report: %w", err)
	}

	return nil
}

func truncate(message string) string {
	const limit = 400
	message = strings.TrimSpace(message)
	if len(message) <= limit {
		return message
	}

	return message[:limit] + "..."
}

func fragmentKey(fragment Fragment) string {
	return fmt.Sprintf("%s:%d:%d", fragment.File, fragment.Start, fragment.End)
}

func fragmentLess(left, right Fragment) bool {
	if left.File != right.File {
		return left.File < right.File
	}
	if left.Start != right.Start {
		return left.Start < right.Start
	}

	return left.End < right.End
}
