package mine

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"lodestone/internal/gitwalk"
	"lodestone/internal/gofunc"
)

type Site struct {
	File string `json:"file"`
	Func string `json:"func"`
	Line int    `json:"line"`
}

type Candidate struct {
	Commit  string  `json:"commit"`
	Parent  string  `json:"parent"`
	Subject string  `json:"subject"`
	Signal  string  `json:"signal"`
	Via     string  `json:"via"`
	Overlap float64 `json:"overlap"`
	A       Site    `json:"a"`
	B       Site    `json:"b"`
	Verdict string  `json:"verdict"`
}

type Options struct {
	MinOverlap   float64
	MinLines     int
	IncludeTests bool
}

type snapshot struct {
	parent map[string]gofunc.Func
	child  map[string]gofunc.Func
}

func Commit(repo string, commit gitwalk.Commit, opts Options) ([]Candidate, error) {
	files, err := gitwalk.ChangedGoFiles(repo, commit.Parent, commit.Hash)
	if err != nil {
		return nil, fmt.Errorf("diff %s: %w", commit.Hash, err)
	}

	snap := snapshot{parent: map[string]gofunc.Func{}, child: map[string]gofunc.Func{}}
	for _, path := range files {
		if skip(path, opts) {
			continue
		}
		snap.load(repo, commit.Parent, path, snap.parent, opts.MinLines)
		// The child side keeps every function, however short. A body that shrank
		// below the floor still exists, and dropping it here would leave the
		// commit looking like it deleted the function outright.
		snap.load(repo, commit.Hash, path, snap.child, 0)
	}

	var out []Candidate
	seen := map[[2]string]bool{}
	for _, cand := range append(append(snap.inlined(opts), snap.merged(opts)...), snap.extracted(opts)...) {
		key := [2]string{cand.A.File + "::" + cand.A.Func, cand.B.File + "::" + cand.B.Func}
		if seen[key] {
			continue
		}
		seen[key] = true

		cand.Commit, cand.Parent, cand.Subject = commit.Hash, commit.Parent, commit.Subject
		out = append(out, cand)
	}

	slices.SortFunc(out, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(b.Overlap, a.Overlap), cmp.Compare(a.A.Func, b.A.Func), cmp.Compare(a.B.Func, b.B.Func))
	})
	return out, nil
}

func skip(path string, opts Options) bool {
	if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
		return true
	}
	return path == "vendor" || strings.HasPrefix(path, "vendor/") || strings.Contains(path, "/vendor/")
}

func (s snapshot) load(repo, rev, path string, into map[string]gofunc.Func, minLines int) {
	src := gitwalk.Show(repo, rev, path)
	if src == nil || gofunc.IsGenerated(src) {
		return
	}

	ambiguous := map[string]bool{}
	for _, function := range gofunc.Parse(path, src) {
		if len(function.Lines) < minLines {
			continue
		}
		key := function.Key()
		if _, taken := into[key]; taken || ambiguous[key] {
			// Two bodies share one key (repeated init or _ functions);
			// mining either would attribute the wrong body.
			ambiguous[key] = true
			delete(into, key)

			continue
		}
		into[key] = function
	}
}

func (s snapshot) deleted() []gofunc.Func {
	return only(s.parent, s.child)
}

func (s snapshot) added() []gofunc.Func {
	return only(s.child, s.parent)
}

func only(from, other map[string]gofunc.Func) []gofunc.Func {
	var out []gofunc.Func
	for key, fn := range from {
		if _, ok := other[key]; !ok {
			out = append(out, fn)
		}
	}
	slices.SortFunc(out, func(a, b gofunc.Func) int { return cmp.Compare(a.Key(), b.Key()) })
	return out
}

// inlined pairs a function the commit deleted with one that survived it, which
// is the label a developer wrote by hand: they judged the two interchangeable
// enough to keep only one.
func (s snapshot) inlined(opts Options) []Candidate {
	var out []Candidate
	for _, gone := range s.deleted() {
		best, score := gofunc.Func{}, 0.0
		keys := make([]string, 0, len(s.parent))
		for key := range s.parent {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			survivor := s.parent[key]
			if _, alive := s.child[key]; !alive || key == gone.Key() {
				continue
			}
			if score2 := gofunc.Overlap(gone.Lines, survivor.Lines); score2 > score {
				best, score = survivor, score2
			}
		}
		if score >= opts.MinOverlap {
			out = append(out, pair("inline", best.Name, score, gone, best))
		}
	}
	return out
}

// merged pairs functions the commit replaced with a single new one. Two bodies
// absorbed by the same replacement were duplicates in the author's judgement.
func (s snapshot) merged(opts Options) []Candidate {
	gone := s.deleted()
	out := make([]Candidate, 0, len(s.added()))
	for _, fresh := range s.added() {
		var absorbed []gofunc.Func
		weakest := 1.0
		for _, gone := range gone {
			if score := gofunc.Contained(gone.Lines, fresh.Lines); score >= opts.MinOverlap {
				absorbed = append(absorbed, gone)
				weakest = min(weakest, score)
			}
		}
		out = append(out, combinations("merge", fresh.Name, weakest, absorbed)...)
	}
	return out
}

// extracted pairs functions that both shrank into the same new helper. The
// removed lines, not the whole bodies, are what the two shared.
func (s snapshot) extracted(opts Options) []Candidate {
	out := make([]Candidate, 0, len(s.added()))
	for _, helper := range s.added() {
		var shrunk []gofunc.Func
		weakest := 1.0
		for key, before := range s.parent {
			after, alive := s.child[key]
			if !alive {
				continue
			}
			gone := gofunc.Removed(before.Lines, after.Lines)
			if len(gone) < opts.MinLines {
				continue
			}
			if score := gofunc.Contained(gone, helper.Lines); score >= opts.MinOverlap {
				shrunk = append(shrunk, before)
				weakest = min(weakest, score)
			}
		}
		slices.SortFunc(shrunk, func(a, b gofunc.Func) int { return cmp.Compare(a.Key(), b.Key()) })
		out = append(out, combinations("extract", helper.Name, weakest, shrunk)...)
	}
	return out
}

func combinations(signal, via string, score float64, funcs []gofunc.Func) []Candidate {
	const leastForAPair = 2
	if len(funcs) < leastForAPair {
		return nil
	}

	var out []Candidate
	for i := range funcs {
		for j := i + 1; j < len(funcs); j++ {
			out = append(out, pair(signal, via, score, funcs[i], funcs[j]))
		}
	}
	return out
}

func pair(signal, via string, score float64, left, right gofunc.Func) Candidate {
	if left.Key() > right.Key() {
		left, right = right, left
	}
	return Candidate{
		Signal:  signal,
		Via:     via,
		Overlap: score,
		A:       Site{File: left.File, Func: left.Name, Line: left.Line},
		B:       Site{File: right.File, Func: right.Name, Line: right.Line},
	}
}
