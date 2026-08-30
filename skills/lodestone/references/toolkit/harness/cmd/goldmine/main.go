package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"

	"lodestone/internal/gitwalk"
	"lodestone/internal/mine"
)

const (
	defaultMinOverlap = 0.5
	defaultMinLines   = 6
	hashDigits        = 8
)

var errRepoRequired = errors.New("-repo is required")

func main() {
	repo := flag.String("repo", "", "target Go repository (required)")
	limit := flag.Int("limit", 0, "newest commits to walk; 0 walks all")
	out := flag.String("out", "", "JSONL destination; empty writes to stdout")
	minOverlap := flag.Float64("min-overlap", defaultMinOverlap, "least line overlap a pair must show")
	minLines := flag.Int("min-lines", defaultMinLines, "shortest function body considered")
	includeTests := flag.Bool("include-tests", false, "keep _test.go functions")
	flag.Parse()

	if err := run(*repo, *limit, *out, mine.Options{
		MinOverlap:   *minOverlap,
		MinLines:     *minLines,
		IncludeTests: *includeTests,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "goldmine:", err)
		os.Exit(1)
	}
}

func run(repo string, limit int, out string, opts mine.Options) error {
	if repo == "" {
		return errRepoRequired
	}

	commits, err := gitwalk.Commits(repo, limit)
	if err != nil {
		return fmt.Errorf("walk %s: %w", repo, err)
	}

	sink := os.Stdout
	if out != "" {
		file, err := os.Create(out) //nolint:gosec // the destination comes from the operator's own flag
		if err != nil {
			return fmt.Errorf("create %s: %w", out, err)
		}
		defer func() { _ = file.Close() }()
		sink = file
	}

	encoder := json.NewEncoder(sink)
	counts := map[string]int{}
	for _, commit := range commits {
		candidates, err := mine.Commit(repo, commit, opts)
		if err != nil {
			return fmt.Errorf("commit %s: %w", commit.Hash[:hashDigits], err)
		}
		for _, candidate := range candidates {
			if err := encoder.Encode(candidate); err != nil {
				return fmt.Errorf("write candidate: %w", err)
			}
			counts[candidate.Signal]++
		}
	}

	fmt.Fprintf(os.Stderr, "walked %d commits\n", len(commits))
	total := 0
	for _, signal := range slices.Sorted(maps.Keys(counts)) {
		fmt.Fprintf(os.Stderr, "  %-8s %d\n", signal, counts[signal])
		total += counts[signal]
	}
	fmt.Fprintf(os.Stderr, "  %-8s %d unreviewed candidates\n", "total", total)
	return nil
}
