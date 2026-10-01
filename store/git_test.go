package store

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDetectGitWorktreeUsesMainRepoName(t *testing.T) {
	tmp := t.TempDir()
	main := filepath.Join(tmp, "mainrepo")
	runGit(t, tmp, "init", "-q", "-b", "main", main)
	runGit(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(tmp, "mainrepo.worktrees", "KEY-1")
	runGit(t, main, "worktree", "add", "-q", "-b", "KEY-1-thing", wt)

	gc, err := DetectGit(wt)
	if err != nil {
		t.Fatalf("DetectGit(worktree): %v", err)
	}
	if gc.RepoName != "mainrepo" || gc.Branch != "KEY-1-thing" {
		t.Errorf("worktree context = %+v, want repo mainrepo on KEY-1-thing", gc)
	}
	if gc.RepoRoot != wt {
		t.Errorf("RepoRoot = %s, want %s", gc.RepoRoot, wt)
	}

	gc, err = DetectGit(main)
	if err != nil {
		t.Fatalf("DetectGit(main): %v", err)
	}
	if gc.RepoName != "mainrepo" || gc.Branch != "main" {
		t.Errorf("main context = %+v, want repo mainrepo on main", gc)
	}
}

func TestDetectGitOutsideRepo(t *testing.T) {
	if _, err := DetectGit(t.TempDir()); !errors.Is(err, ErrNotInRepo) {
		t.Errorf("DetectGit outside repo err = %v, want ErrNotInRepo", err)
	}
}
