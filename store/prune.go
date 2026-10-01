package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StaleBranch describes a branch namespace directory whose branch no longer
// exists in the repository.
type StaleBranch struct {
	Branch string `json:"branch"` // branch path, slash-separated
	Dir    string `json:"dir"`    // absolute directory
	Files  int    `json:"files"`  // number of files under Dir
}

// StaleBranches returns the branch namespaces stored for repoName whose branch
// is absent from liveBranches (typically the output of `git for-each-ref
// refs/heads`). A missing branches directory yields no results.
func (s *Store) StaleBranches(repoName string, liveBranches []string) ([]StaleBranch, error) {
	repoDir := filepath.Join(s.BranchesDir(), repoName)
	info, err := os.Stat(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	live := make(map[string]bool, len(liveBranches))
	for _, b := range liveBranches {
		if b = strings.TrimSpace(b); b != "" {
			live[b] = true
		}
	}

	var stale []StaleBranch
	if err := s.walkStale(repoDir, "", live, &stale); err != nil {
		return nil, err
	}
	return stale, nil
}

type branchClass int

const (
	classStale    branchClass = iota // no live branch maps into this dir
	classLive                        // a live branch, or inside a live branch's file tree
	classAncestor                    // an intermediate dir above a live branch (e.g. feature/ for feature/foo)
)

// classifyBranchPath decides how a directory (identified by its branch-relative
// path) relates to the set of live branches.
func classifyBranchPath(rel string, live map[string]bool) branchClass {
	for b := range live {
		if rel == b || strings.HasPrefix(rel, b+"/") {
			return classLive
		}
	}
	for b := range live {
		if strings.HasPrefix(b, rel+"/") {
			return classAncestor
		}
	}
	return classStale
}

func (s *Store) walkStale(dir, rel string, live map[string]bool, out *[]StaleBranch) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		childRel := e.Name()
		if rel != "" {
			childRel = rel + "/" + e.Name()
		}
		childDir := filepath.Join(dir, e.Name())
		switch classifyBranchPath(childRel, live) {
		case classLive:
			// active branch subtree; keep it.
		case classAncestor:
			if err := s.walkStale(childDir, childRel, live, out); err != nil {
				return err
			}
		case classStale:
			n, err := countFiles(childDir)
			if err != nil {
				return err
			}
			*out = append(*out, StaleBranch{Branch: childRel, Dir: childDir, Files: n})
		}
	}
	return nil
}

func countFiles(dir string) (int, error) {
	n := 0
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n, err
}

// PruneBranch removes a single branch namespace directory (and its contents)
// for repoName, then prunes now-empty parent directories up to the repo's
// branches root.
func (s *Store) PruneBranch(repoName, branch string) error {
	repoBase := filepath.Join(s.BranchesDir(), repoName)
	dir := filepath.Join(repoBase, filepath.FromSlash(branch))
	rel, err := filepath.Rel(repoBase, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes branch namespace", ErrBadPath, branch)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	pruneEmptyDirs(filepath.Dir(dir), repoBase)
	return nil
}
