package store

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInRepo is returned when git context is requested outside a repository.
var ErrNotInRepo = errors.New("not inside a git repository")

// GitContext describes the repository containing a working directory.
type GitContext struct {
	RepoRoot string // absolute path to the repo top level
	RepoName string // basename of RepoRoot
	Branch   string // current branch, or detached/<short-sha> when detached
}

// DetectGit resolves the git repository context for cwd by shelling out to git.
func DetectGit(cwd string) (*GitContext, error) {
	root, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotInRepo
	}
	branch := detectBranch(cwd)
	return &GitContext{
		RepoRoot: root,
		RepoName: filepath.Base(root),
		Branch:   branch,
	}, nil
}

// detectBranch returns the current branch name, or a "detached/<short-sha>"
// placeholder when HEAD is detached.
func detectBranch(cwd string) string {
	if branch, err := gitOutput(cwd, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && branch != "" {
		return branch
	}
	if sha, err := gitOutput(cwd, "rev-parse", "--short", "HEAD"); err == nil && sha != "" {
		return "detached/" + sha
	}
	return "detached/unknown"
}

// LiveBranches returns the local branch names in the repository at cwd, as
// reported by `git for-each-ref refs/heads`.
func LiveBranches(cwd string) ([]string, error) {
	out, err := gitOutput(cwd, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func gitOutput(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
