package fusion

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"lodestone/internal/analyze"
	"lodestone/internal/detect"
)

var errRRFK = errors.New("RRF k must be positive")

type Retrieved struct {
	Name  string
	Pairs []detect.Pair
}

type Source struct {
	Name   string  `json:"name"`
	Rank   int     `json:"rank"`
	Metric float64 `json:"metric,omitempty"`
}

type Candidate struct {
	analyze.Candidate

	RRFScore float64  `json:"rrfScore"`
	Sources  []Source `json:"sources"`

	LeftSource  string `json:"leftSource,omitempty"`
	RightSource string `json:"rightSource,omitempty"`
}

type Result struct {
	Candidates []Candidate
	Counts     map[string]int
}

type entry struct {
	candidate analyze.Candidate
	sources   map[string]Source
}

func Fuse(
	root string,
	functions []analyze.Function,
	semantic []analyze.Candidate,
	retrieved []Retrieved,
	rrfK float64,
	limit int,
	includeTests bool,
) (Result, error) {
	if rrfK <= 0 {
		return Result{}, errRRFK
	}
	entries := semanticEntries(semantic)
	for _, source := range retrieved {
		addPairs(entries, root, functions, source, includeTests)
	}
	candidates := makeCandidates(entries, rrfK)
	sortCandidates(candidates)
	counts := countSources(candidates)
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return Result{Candidates: candidates, Counts: counts}, nil
}

func semanticEntries(semantic []analyze.Candidate) map[string]*entry {
	entries := make(map[string]*entry)
	for rank, candidate := range semantic {
		key := pairKey(candidate.Left.ID, candidate.Right.ID)
		entries[key] = &entry{
			candidate: candidate,
			sources: map[string]Source{
				"semantic": {Name: "semantic", Rank: rank + 1, Metric: candidate.SemanticScore},
			},
		}
	}

	return entries
}

func addPairs(
	entries map[string]*entry,
	root string,
	functions []analyze.Function,
	source Retrieved,
	includeTests bool,
) {
	rank := 0
	for _, pair := range source.Pairs {
		if !includeTests && (isTest(pair.First.File) || isTest(pair.Second.File)) {
			continue
		}
		left, leftOK := bestFunction(root, functions, pair.First)
		right, rightOK := bestFunction(root, functions, pair.Second)
		if !leftOK || !rightOK || left.ID == right.ID {
			continue
		}
		rank++
		add(entries, left, right, Source{Name: source.Name, Rank: rank, Metric: pair.Metric})
	}
}

func countSources(candidates []Candidate) map[string]int {
	counts := make(map[string]int)
	for _, candidate := range candidates {
		for _, source := range candidate.Sources {
			counts[source.Name]++
		}
	}

	return counts
}

func makeCandidates(entries map[string]*entry, rrfK float64) []Candidate {
	result := make([]Candidate, 0, len(entries))
	for _, item := range entries {
		candidate := Candidate{Candidate: item.candidate}
		for _, source := range item.sources {
			candidate.Sources = append(candidate.Sources, source)
		}
		sort.Slice(candidate.Sources, func(left, right int) bool {
			return candidate.Sources[left].Name < candidate.Sources[right].Name
		})
		// Summed after sorting: float addition is not associative, and map
		// order would let one-ulp noise reorder equal-score candidates.
		for _, source := range candidate.Sources {
			candidate.RRFScore += 1 / (rrfK + float64(source.Rank))
		}
		result = append(result, candidate)
	}

	return result
}

func sortCandidates(result []Candidate) {
	sort.Slice(result, func(left, right int) bool {
		if result[left].RRFScore != result[right].RRFScore {
			return result[left].RRFScore > result[right].RRFScore
		}
		if len(result[left].Sources) != len(result[right].Sources) {
			return len(result[left].Sources) > len(result[right].Sources)
		}
		leftLocality := analyze.LocalityRank(result[left].Locality)
		rightLocality := analyze.LocalityRank(result[right].Locality)
		if leftLocality != rightLocality {
			return leftLocality < rightLocality
		}
		leftEvidence := result[left].StructureScore + result[left].CallScore
		rightEvidence := result[right].StructureScore + result[right].CallScore
		if leftEvidence != rightEvidence {
			return leftEvidence > rightEvidence
		}
		if result[left].SemanticScore != result[right].SemanticScore {
			return result[left].SemanticScore > result[right].SemanticScore
		}

		return pairKey(result[left].Left.ID, result[left].Right.ID) <
			pairKey(result[right].Left.ID, result[right].Right.ID)
	})
}

func add(entries map[string]*entry, left, right analyze.Function, source Source) {
	if left.ID > right.ID {
		left, right = right, left
	}
	key := pairKey(left.ID, right.ID)
	item, ok := entries[key]
	if !ok {
		item = &entry{candidate: analyze.Compare(left, right, 0), sources: make(map[string]Source)}
		entries[key] = item
	}
	if current, ok := item.sources[source.Name]; !ok || source.Rank < current.Rank {
		item.sources[source.Name] = source
	}
}

func bestFunction(root string, functions []analyze.Function, target detect.Fragment) (analyze.Function, bool) {
	name := filepath.ToSlash(target.File)
	if filepath.IsAbs(target.File) {
		relative, err := filepath.Rel(root, target.File)
		if err != nil {
			return analyze.Function{}, false
		}
		name = filepath.ToSlash(relative)
	}
	bestOverlap := 0
	bestLines := 0
	var best analyze.Function
	for _, function := range functions {
		if function.File != name {
			continue
		}
		overlap := min(function.EndLine, target.End) - max(function.Line, target.Start) + 1
		if overlap <= 0 {
			continue
		}
		if overlap > bestOverlap || (overlap == bestOverlap && (bestLines == 0 || function.Lines < bestLines)) {
			best = function
			bestOverlap = overlap
			bestLines = function.Lines
		}
	}

	return best, bestOverlap > 0
}

func pairKey(left, right string) string {
	if left > right {
		left, right = right, left
	}

	return left + "\x00" + right
}

func isTest(path string) bool {
	return strings.HasSuffix(filepath.Base(path), "_test.go")
}
