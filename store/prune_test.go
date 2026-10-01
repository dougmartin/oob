package store

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestStaleBranchesAndPrune(t *testing.T) {
	s := newTestStore(t)
	repo := "myrepo"

	// Seed branch namespaces: two live, two stale (one nested).
	mustWrite(t, s, NamespaceBranch, "notes.md", "x")       // branch feature/foo (from newTestStore)
	seedBranch(t, s, repo, "main", "todo.txt")              // live
	seedBranch(t, s, repo, "feature/foo", "extra/deep.txt") // live (same branch as default, nested file)
	seedBranch(t, s, repo, "old-thing", "scratch.txt")      // stale
	seedBranch(t, s, repo, "feature/dead", "notes.txt")     // stale, shares "feature/" ancestor with a live branch

	live := []string{"main", "feature/foo"}
	stale, err := s.StaleBranches(repo, live)
	if err != nil {
		t.Fatalf("StaleBranches: %v", err)
	}

	got := make([]string, len(stale))
	for i, sb := range stale {
		got[i] = sb.Branch
	}
	sort.Strings(got)
	want := []string{"feature/dead", "old-thing"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("stale = %v, want %v", got, want)
	}

	// A live branch that shares the feature/ ancestor must be preserved by prune.
	for _, sb := range stale {
		if err := s.PruneBranch(repo, sb.Branch); err != nil {
			t.Fatalf("PruneBranch(%s): %v", sb.Branch, err)
		}
	}
	assertExists(t, s, repo, "feature/foo")
	assertExists(t, s, repo, "main")
	assertGone(t, s, repo, "old-thing")
	assertGone(t, s, repo, "feature/dead")
	// The shared "feature" ancestor dir must survive (feature/foo still lives there).
	assertExists(t, s, repo, "feature")
}

func TestStaleBranchesNoBranchesDir(t *testing.T) {
	s := newTestStore(t)
	stale, err := s.StaleBranches("neverseen", []string{"main"})
	if err != nil || stale != nil {
		t.Fatalf("StaleBranches empty = %v err=%v", stale, err)
	}
}

func seedBranch(t *testing.T, s *Store, repo, branch, relFile string) {
	t.Helper()
	dir := filepath.Join(s.BranchesDir(), repo, filepath.FromSlash(branch))
	path := filepath.Join(dir, filepath.FromSlash(relFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, s *Store, repo, branch string) {
	t.Helper()
	dir := filepath.Join(s.BranchesDir(), repo, filepath.FromSlash(branch))
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected %s to exist: %v", branch, err)
	}
}

func assertGone(t *testing.T, s *Store, repo, branch string) {
	t.Helper()
	dir := filepath.Join(s.BranchesDir(), repo, filepath.FromSlash(branch))
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected %s gone, stat err=%v", branch, err)
	}
}
