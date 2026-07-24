package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newTestStore returns a Store rooted at a temp dir with a fixed git context.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	s := &Store{
		Home: home,
		git: func(string) (*GitContext, error) {
			return &GitContext{RepoRoot: "/x/myrepo", RepoName: "myrepo", Branch: "feature/foo"}, nil
		},
		cwd: func() (string, error) { return "/x/myrepo", nil },
	}
	if err := s.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	return s
}

func TestNamespaceDirs(t *testing.T) {
	s := newTestStore(t)
	cases := map[string]string{
		NamespaceGlobal: filepath.Join(s.Home, "global"),
		NamespaceRepo:   filepath.Join(s.Home, "repos", "myrepo"),
		NamespaceBranch: filepath.Join(s.Home, "branches", "myrepo", "feature", "foo"),
	}
	for ns, want := range cases {
		got, err := s.NamespaceDir(ns, Ref{})
		if err != nil {
			t.Fatalf("NamespaceDir(%s): %v", ns, err)
		}
		if got != want {
			t.Errorf("NamespaceDir(%s) = %s, want %s", ns, got, want)
		}
	}
	if _, err := s.NamespaceDir("bogus", Ref{}); !errors.Is(err, ErrBadNamespace) {
		t.Errorf("bogus namespace error = %v, want ErrBadNamespace", err)
	}
}

func TestNamespaceDirOverrides(t *testing.T) {
	s := newTestStore(t)
	// Repo override targets a different repo without touching git.
	got, err := s.NamespaceDir(NamespaceRepo, Ref{Repo: "other"})
	if err != nil {
		t.Fatalf("repo override: %v", err)
	}
	if want := filepath.Join(s.Home, "repos", "other"); got != want {
		t.Errorf("repo override = %s, want %s", got, want)
	}
	// Branch override on a different repo, with a slashed branch.
	got, err = s.NamespaceDir(NamespaceBranch, Ref{Repo: "other", Branch: "bugfix/x"})
	if err != nil {
		t.Fatalf("branch override: %v", err)
	}
	if want := filepath.Join(s.Home, "branches", "other", "bugfix", "x"); got != want {
		t.Errorf("branch override = %s, want %s", got, want)
	}
	// Branch override with repo still detected from git context.
	got, err = s.NamespaceDir(NamespaceBranch, Ref{Branch: "release"})
	if err != nil {
		t.Fatalf("partial override: %v", err)
	}
	if want := filepath.Join(s.Home, "branches", "myrepo", "release"); got != want {
		t.Errorf("partial override = %s, want %s", got, want)
	}
	// Overrides let repo/branch namespaces resolve even outside a git repo.
	s.git = func(string) (*GitContext, error) { return nil, ErrNotInRepo }
	if _, err := s.NamespaceDir(NamespaceRepo, Ref{Repo: "x"}); err != nil {
		t.Errorf("override outside repo: %v", err)
	}
	if _, err := s.NamespaceDir(NamespaceRepo, Ref{}); !errors.Is(err, ErrNotInRepo) {
		t.Errorf("no override outside repo err = %v, want ErrNotInRepo", err)
	}
}

func TestWriteReadListDelete(t *testing.T) {
	s := newTestStore(t)

	created, err := s.Write(NamespaceRepo, "a/b/c.txt", "hi", false, Ref{})
	if err != nil || !created {
		t.Fatalf("Write created=%v err=%v", created, err)
	}
	// Overwrite reports created=false.
	if created, err = s.Write(NamespaceRepo, "a/b/c.txt", "hey", false, Ref{}); err != nil || created {
		t.Fatalf("overwrite created=%v err=%v", created, err)
	}
	got, err := s.Read(NamespaceRepo, "a/b/c.txt", Ref{})
	if err != nil || got != "hey" {
		t.Fatalf("Read = %q err=%v", got, err)
	}

	// create_only on an existing file fails.
	if _, err := s.Write(NamespaceRepo, "a/b/c.txt", "x", true, Ref{}); !errors.Is(err, ErrExists) {
		t.Errorf("create_only error = %v, want ErrExists", err)
	}

	entries, err := s.List(NamespaceRepo, "", Ref{})
	if err != nil || len(entries) != 1 || entries[0].Path != "a/b/c.txt" {
		t.Fatalf("List = %+v err=%v", entries, err)
	}

	if err := s.Delete(NamespaceRepo, "a/b/c.txt", Ref{}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Empty parent dirs pruned back to the namespace root.
	if _, err := os.Stat(filepath.Join(s.Home, "repos", "myrepo", "a")); !os.IsNotExist(err) {
		t.Errorf("expected pruned dir, stat err=%v", err)
	}
	if _, err := s.Read(NamespaceRepo, "a/b/c.txt", Ref{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Read after delete err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(NamespaceRepo, "a/b/c.txt", Ref{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing err = %v, want ErrNotFound", err)
	}
}

func TestPathTraversalContained(t *testing.T) {
	s := newTestStore(t)
	// A traversal attempt is clamped inside the namespace, never escaping.
	if _, err := s.Write(NamespaceGlobal, "../../evil.txt", "x", false, Ref{}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Home, "global", "evil.txt")); err != nil {
		t.Errorf("expected clamped file inside global, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Home, "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("file escaped namespace: %v", err)
	}
	// An empty path is rejected outright.
	if _, err := s.Write(NamespaceGlobal, "   ", "x", false, Ref{}); !errors.Is(err, ErrBadPath) {
		t.Errorf("empty path err = %v, want ErrBadPath", err)
	}
}

func TestAbsPath(t *testing.T) {
	s := newTestStore(t)
	// Resolves without the file existing, so callers can plan a new file.
	got, err := s.AbsPath(NamespaceBranch, "notes/plan.md", Ref{})
	if err != nil {
		t.Fatalf("AbsPath: %v", err)
	}
	want := filepath.Join(s.Home, "branches", "myrepo", "feature", "foo", "notes", "plan.md")
	if got != want {
		t.Errorf("AbsPath = %s, want %s", got, want)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("AbsPath created something on disk: %v", err)
	}
	// Overrides and traversal guards behave as they do elsewhere.
	got, err = s.AbsPath(NamespaceRepo, "a.txt", Ref{Repo: "other"})
	if err != nil {
		t.Fatalf("AbsPath override: %v", err)
	}
	if want := filepath.Join(s.Home, "repos", "other", "a.txt"); got != want {
		t.Errorf("AbsPath override = %s, want %s", got, want)
	}
	if _, err := s.AbsPath(NamespaceGlobal, "  ", Ref{}); !errors.Is(err, ErrBadPath) {
		t.Errorf("empty path err = %v, want ErrBadPath", err)
	}
}

func TestSearch(t *testing.T) {
	s := newTestStore(t)
	mustWrite(t, s, NamespaceGlobal, "g.txt", "alpha beta\nGAMMA line\n")
	mustWrite(t, s, NamespaceRepo, "r.txt", "gamma here\n")

	matches, err := s.Search("", "gamma", 0, Ref{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Search all = %d matches, want 2: %+v", len(matches), matches)
	}

	// Each match carries an absolute path callers can edit directly.
	for _, m := range matches {
		want, err := s.AbsPath(m.Namespace, m.Path, Ref{})
		if err != nil {
			t.Fatalf("AbsPath(%s/%s): %v", m.Namespace, m.Path, err)
		}
		if m.AbsPath != want {
			t.Errorf("match AbsPath = %s, want %s", m.AbsPath, want)
		}
	}

	// Scoped search + maxResults cap.
	scoped, err := s.Search(NamespaceGlobal, "gamma", 0, Ref{})
	if err != nil || len(scoped) != 1 || scoped[0].Namespace != NamespaceGlobal {
		t.Fatalf("scoped search = %+v err=%v", scoped, err)
	}

	capped, err := s.Search("", "gamma", 1, Ref{})
	if err != nil || len(capped) != 1 {
		t.Fatalf("capped search = %+v err=%v", capped, err)
	}
}

func TestListMissingNamespaceIsEmpty(t *testing.T) {
	s := newTestStore(t)
	entries, err := s.List(NamespaceBranch, "", Ref{})
	if err != nil || len(entries) != 0 {
		t.Fatalf("List missing = %+v err=%v", entries, err)
	}
}

func TestContextOutsideRepo(t *testing.T) {
	s := newTestStore(t)
	s.git = func(string) (*GitContext, error) { return nil, ErrNotInRepo }

	c, err := s.Context()
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if c.InGit {
		t.Errorf("InGit = true, want false")
	}
	// repo/branch operations surface the not-in-repo error.
	if _, err := s.NamespaceDir(NamespaceRepo, Ref{}); !errors.Is(err, ErrNotInRepo) {
		t.Errorf("NamespaceDir(repo) err = %v, want ErrNotInRepo", err)
	}
	// all-namespace search skips repo/branch and still searches global.
	mustWrite(t, s, NamespaceGlobal, "g.txt", "findme\n")
	matches, err := s.Search("", "findme", 0, Ref{})
	if err != nil || len(matches) != 1 {
		t.Fatalf("search outside repo = %+v err=%v", matches, err)
	}
}

func mustWrite(t *testing.T, s *Store, ns, path, content string) {
	t.Helper()
	if _, err := s.Write(ns, path, content, false, Ref{}); err != nil {
		t.Fatalf("Write(%s/%s): %v", ns, path, err)
	}
}
