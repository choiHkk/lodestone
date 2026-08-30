package mine_test

import (
	"os"
	"os/exec"
	"testing"

	"lodestone/internal/gitwalk"
	"lodestone/internal/mine"
)

const body = `	total := 0
	for _, item := range items {
		total += item
	}
	if total < 0 {
		total = 0
	}
`

func header(name string) string {
	return "func " + name + "(items []int) int {\n"
}

func twoDuplicates() string {
	return "package alpha\n\n" +
		header("Alpha") + body + "\treturn total\n}\n\n" +
		header("Beta") + body + "\treturn total\n}\n"
}

func oneSurvivor() string {
	return "package alpha\n\n" + header("Alpha") + body + "\treturn total\n}\n"
}

func twoSharers() string {
	return "package alpha\n\n" +
		header("One") + body + "\treturn total + 1\n}\n\n" +
		header("Two") + body + "\treturn total + 2\n}\n"
}

func helperExtracted() string {
	return "package alpha\n\n" +
		header("Shared") + body + "\treturn total\n}\n\n" +
		"func One(items []int) int {\n\treturn Shared(items) + 1\n}\n\n" +
		"func Two(items []int) int {\n\treturn Shared(items) + 2\n}\n"
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	//nolint:gosec // the arguments are this test's own literals
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func repoWith(t *testing.T, revisions ...string) (string, gitwalk.Commit) {
	t.Helper()

	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "bench@example.com")
	git(t, dir, "config", "user.name", "bench")
	git(t, dir, "config", "commit.gpgsign", "false")

	for i, source := range revisions {
		if err := os.WriteFile(dir+"/alpha.go", []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "add", "alpha.go")
		git(t, dir, "commit", "-q", "-m", string(rune('a'+i)))
	}

	commits, err := gitwalk.Commits(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("want the newest commit, got %d", len(commits))
	}
	return dir, commits[0]
}

func mined(t *testing.T, revisions ...string) []mine.Candidate {
	t.Helper()

	dir, commit := repoWith(t, revisions...)
	candidates, err := mine.Commit(dir, commit, mine.Options{MinOverlap: 0.6, MinLines: 6})
	if err != nil {
		t.Fatal(err)
	}
	return candidates
}

func TestDeletedDuplicateIsPairedWithItsSurvivor(t *testing.T) {
	t.Parallel()
	candidates := mined(t, twoDuplicates(), oneSurvivor())

	if len(candidates) != 1 {
		t.Fatalf("want one candidate, got %d: %+v", len(candidates), candidates)
	}
	got := candidates[0]
	if got.Signal != "inline" {
		t.Errorf("signal = %q, want inline", got.Signal)
	}
	if got.A.Func != "Alpha" || got.B.Func != "Beta" {
		t.Errorf("pair = (%s, %s), want (Alpha, Beta)", got.A.Func, got.B.Func)
	}
	if got.Via != "Alpha" {
		t.Errorf("via = %q, want Alpha", got.Via)
	}
}

func TestFunctionsSharingAnExtractedHelperArePaired(t *testing.T) {
	t.Parallel()
	candidates := mined(t, twoSharers(), helperExtracted())

	if len(candidates) != 1 {
		t.Fatalf("want one candidate, got %d: %+v", len(candidates), candidates)
	}
	got := candidates[0]
	if got.Signal != "extract" {
		t.Errorf("signal = %q, want extract", got.Signal)
	}
	if got.A.Func != "One" || got.B.Func != "Two" {
		t.Errorf("pair = (%s, %s), want (One, Two)", got.A.Func, got.B.Func)
	}
	if got.Via != "Shared" {
		t.Errorf("via = %q, want Shared", got.Via)
	}
}

func TestAShrunkFunctionIsNotReportedAsDeleted(t *testing.T) {
	t.Parallel()
	for _, candidate := range mined(t, twoSharers(), helperExtracted()) {
		if candidate.Signal == "inline" {
			t.Errorf("shrinking One and Two into Shared was mined as %q", candidate.Signal)
		}
	}
}

func TestAnUnrelatedEditYieldsNothing(t *testing.T) {
	t.Parallel()
	edited := "package alpha\n\n" + header("Alpha") + body + "\treturn total * 3\n}\n"

	if candidates := mined(t, oneSurvivor(), edited); len(candidates) != 0 {
		t.Errorf("want no candidates from an in-place edit, got %+v", candidates)
	}
}
