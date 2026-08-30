package gitwalk

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Commit struct {
	Hash    string
	Parent  string
	Subject string
}

func run(repo string, args ...string) ([]byte, error) {
	//nolint:gosec // the repository comes from the operator's own flag
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", repo}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

func Commits(repo string, limit int) ([]Commit, error) {
	args := []string{"log", "--no-merges", "--format=%H %P%x00%s"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", limit))
	}
	out, err := run(repo, args...)
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		hashes, subject, _ := strings.Cut(line, "\x00")
		fields := strings.Fields(hashes)
		// One hash and one parent; a root commit has no prior state to label
		// against, and a merge was excluded by --no-merges.
		const hashAndParent = 2
		if len(fields) != hashAndParent {
			continue
		}
		commits = append(commits, Commit{Hash: fields[0], Parent: fields[1], Subject: subject})
	}
	return commits, nil
}

func ChangedGoFiles(repo, parent, child string) ([]string, error) {
	out, err := run(repo, "-c", "core.quotePath=false", "diff", "--name-only", "--no-renames", parent, child, "--", "*.go")
	if err != nil {
		return nil, err
	}

	var files []string
	for path := range strings.SplitSeq(strings.TrimRight(string(out), "\n"), "\n") {
		if path != "" {
			files = append(files, path)
		}
	}
	return files, nil
}

// Show returns nil without an error when the path does not exist at rev, which
// is the ordinary case for a file added or deleted by the commit under test.
func Show(repo, rev, path string) []byte {
	out, err := run(repo, "show", rev+":"+path)
	if err != nil {
		return nil
	}
	return out
}
