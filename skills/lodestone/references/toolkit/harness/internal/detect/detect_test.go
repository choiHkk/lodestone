package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	alphaDir = "internal/alpha"
	betaDir  = "internal/beta"
)

func TestDirectoriesKeepsSiblingsApart(t *testing.T) {
	t.Parallel()

	got := Directories([]string{
		alphaDir + "/one.go",
		alphaDir + "/two.go",
		betaDir + "/one.go",
	})
	want := []string{alphaDir, betaDir}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDirectoriesCollapsesDescendantsIntoParent(t *testing.T) {
	t.Parallel()

	got := Directories([]string{
		alphaDir + "/root.go",
		alphaDir + "/one/leaf.go",
		alphaDir + "/two/leaf.go",
	})
	if !slices.Equal(got, []string{alphaDir}) {
		t.Fatalf("got %v, want [%s]", got, alphaDir)
	}
}

func TestDirectoriesCollapsesRepositoryRoot(t *testing.T) {
	t.Parallel()

	got := Directories([]string{"main.go", "internal/a/a.go"})
	if !slices.Equal(got, []string{"."}) {
		t.Fatalf("got %v, want [.]", got)
	}
}

func TestAvailableRejectsMissingAndNonExecutable(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	plain := filepath.Join(directory, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "tool")
	//nolint:gosec // the executable bit is the subject of this test
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		binary string
		want   bool
	}{
		"empty":          {binary: "", want: false},
		"missing":        {binary: filepath.Join(directory, "absent"), want: false},
		"directory":      {binary: directory, want: false},
		"not executable": {binary: plain, want: false},
		"executable":     {binary: executable, want: true},
	} {
		if got := Available(testCase.binary); got != testCase.want {
			t.Fatalf("%s: got %v, want %v", name, got, testCase.want)
		}
	}
}
