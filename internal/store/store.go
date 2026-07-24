// Package store implements the on-disk CRUD layer for oob (out-of-band) files.
//
// Files live under a base directory (~/oob by default) split into three
// namespaces:
//
//	global   -> <home>/global/...
//	repo     -> <home>/repos/<repo>/...
//	branch   -> <home>/branches/<repo>/<branch>/...
//
// The repo and branch namespaces are resolved from the git repository
// containing the current working directory.
package store

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Namespace identifiers accepted by the store.
const (
	NamespaceGlobal = "global"
	NamespaceRepo   = "repo"
	NamespaceBranch = "branch"
)

// Namespaces lists the valid namespace identifiers.
var Namespaces = []string{NamespaceGlobal, NamespaceRepo, NamespaceBranch}

// Store resolves oob file paths under a base home directory.
type Store struct {
	// Home is the base directory for all namespaces (e.g. ~/oob).
	Home string
	// git detects repository context; overridable in tests.
	git func(cwd string) (*GitContext, error)
	// cwd returns the working directory used for git detection.
	cwd func() (string, error)
}

// New creates a Store rooted at the oob home directory. The home is taken from
// the OOB_HOME environment variable, falling back to ~/oob.
func New() (*Store, error) {
	home := os.Getenv("OOB_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		home = filepath.Join(userHome, "oob")
	}
	return &Store{Home: home, git: DetectGit, cwd: os.Getwd}, nil
}

// GlobalDir, ReposDir and BranchesDir return the top-level namespace roots.
func (s *Store) GlobalDir() string   { return filepath.Join(s.Home, "global") }
func (s *Store) ReposDir() string    { return filepath.Join(s.Home, "repos") }
func (s *Store) BranchesDir() string { return filepath.Join(s.Home, "branches") }

// EnsureLayout creates the top-level namespace directories if they do not
// already exist.
func (s *Store) EnsureLayout() error {
	for _, dir := range []string{s.GlobalDir(), s.ReposDir(), s.BranchesDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// Context reports the resolved repo and branch for the current working
// directory. Fields are empty when not inside a git repository.
type Context struct {
	Home     string `json:"home"`
	RepoName string `json:"repo,omitempty"`
	Branch   string `json:"branch,omitempty"`
	RepoRoot string `json:"repo_root,omitempty"`
	InGit    bool   `json:"in_git"`
}

// Context resolves the current git context for reporting.
func (s *Store) Context() (*Context, error) {
	c := &Context{Home: s.Home}
	gc, err := s.gitContext()
	if err != nil {
		if errors.Is(err, ErrNotInRepo) {
			return c, nil
		}
		return nil, err
	}
	c.InGit = true
	c.RepoName = gc.RepoName
	c.Branch = gc.Branch
	c.RepoRoot = gc.RepoRoot
	return c, nil
}

func (s *Store) gitContext() (*GitContext, error) {
	wd, err := s.cwd()
	if err != nil {
		return nil, fmt.Errorf("resolving working directory: %w", err)
	}
	return s.git(wd)
}

// Ref optionally overrides the git-detected repo and/or branch used to resolve
// the repo and branch namespaces. Empty fields fall back to detection from the
// server's working directory, so the zero Ref means "detect everything".
type Ref struct {
	Repo   string
	Branch string
}

// resolveRef determines the repo and branch names for a namespace, honouring
// any overrides in ref and falling back to git detection for the rest. It only
// touches git when a needed value was not supplied.
func (s *Store) resolveRef(namespace string, ref Ref) (repo, branch string, err error) {
	repo = strings.TrimSpace(ref.Repo)
	branch = strings.TrimSpace(ref.Branch)
	needRepo := namespace == NamespaceRepo || namespace == NamespaceBranch
	needBranch := namespace == NamespaceBranch
	if (needRepo && repo == "") || (needBranch && branch == "") {
		gc, gerr := s.gitContext()
		if gerr != nil {
			return "", "", gerr
		}
		if repo == "" {
			repo = gc.RepoName
		}
		if branch == "" {
			branch = gc.Branch
		}
	}
	return repo, branch, nil
}

// NamespaceDir returns the directory backing the given namespace, resolving
// git context (or ref overrides) for repo/branch namespaces.
func (s *Store) NamespaceDir(namespace string, ref Ref) (string, error) {
	switch namespace {
	case NamespaceGlobal:
		return s.GlobalDir(), nil
	case NamespaceRepo:
		repo, _, err := s.resolveRef(namespace, ref)
		if err != nil {
			return "", err
		}
		return safeJoin(s.ReposDir(), repo)
	case NamespaceBranch:
		repo, branch, err := s.resolveRef(namespace, ref)
		if err != nil {
			return "", err
		}
		// Branch may contain slashes (e.g. feature/foo); keep them nested.
		return safeJoin(s.BranchesDir(), filepath.Join(repo, filepath.FromSlash(branch)))
	default:
		return "", fmt.Errorf("%w: %q (want one of %s)", ErrBadNamespace, namespace, strings.Join(Namespaces, ", "))
	}
}

// AbsPath returns the absolute filesystem path a namespace-relative path maps
// to. The file need not exist, so callers can use it to decide where to create
// one. Parent directories are not created.
func (s *Store) AbsPath(namespace, path string, ref Ref) (string, error) {
	_, full, err := s.resolve(namespace, path, ref)
	return full, err
}

// Sentinel errors returned by the store.
var (
	ErrBadNamespace = errors.New("invalid namespace")
	ErrBadPath      = errors.New("invalid path")
	ErrNotFound     = errors.New("file not found")
	ErrExists       = errors.New("file already exists")
)

// resolve returns the absolute filesystem path for a namespace-relative path,
// guarding against traversal outside the namespace directory.
func (s *Store) resolve(namespace, rel string, ref Ref) (base, full string, err error) {
	base, err = s.NamespaceDir(namespace, ref)
	if err != nil {
		return "", "", err
	}
	full, err = safeJoin(base, rel)
	if err != nil {
		return "", "", err
	}
	return base, full, nil
}

// safeJoin joins rel onto base, rejecting empty paths and any result that
// escapes base via "..".
func safeJoin(base, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("%w: path is empty", ErrBadPath)
	}
	// Normalise to a base-relative path, dropping any leading separator.
	clean := filepath.Clean("/" + filepath.FromSlash(rel))
	full := filepath.Join(base, clean)
	rp, err := filepath.Rel(base, full)
	if err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes namespace", ErrBadPath, rel)
	}
	return full, nil
}

// Entry describes a file returned by List.
type Entry struct {
	Path string `json:"path"` // namespace-relative, slash-separated
	Size int64  `json:"size"`
}

// List returns the files under a namespace (optionally scoped to subpath),
// recursively, sorted by path. A missing namespace directory yields an empty
// list rather than an error.
func (s *Store) List(namespace, subpath string, ref Ref) ([]Entry, error) {
	base, err := s.NamespaceDir(namespace, ref)
	if err != nil {
		return nil, err
	}
	root := base
	if strings.TrimSpace(subpath) != "" {
		if _, root, err = s.resolve(namespace, subpath, ref); err != nil {
			return nil, err
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		rel, _ := filepath.Rel(base, root)
		return []Entry{{Path: filepath.ToSlash(rel), Size: info.Size()}}, nil
	}

	var entries []Entry
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Path: filepath.ToSlash(rel), Size: fi.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// Read returns the contents of a namespace file.
func (s *Store) Read(namespace, path string, ref Ref) (string, error) {
	_, full, err := s.resolve(namespace, path, ref)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s/%s", ErrNotFound, namespace, path)
		}
		return "", err
	}
	return string(data), nil
}

// Write creates or updates a namespace file, creating parent directories as
// needed. When createOnly is true, it fails if the file already exists.
// It reports whether a new file was created.
func (s *Store) Write(namespace, path, content string, createOnly bool, ref Ref) (created bool, err error) {
	_, full, err := s.resolve(namespace, path, ref)
	if err != nil {
		return false, err
	}
	_, statErr := os.Stat(full)
	existed := statErr == nil
	if createOnly && existed {
		return false, fmt.Errorf("%w: %s/%s", ErrExists, namespace, path)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return false, err
	}
	return !existed, nil
}

// Delete removes a namespace file and prunes now-empty parent directories up to
// (but not including) the namespace root.
func (s *Store) Delete(namespace, path string, ref Ref) error {
	base, full, err := s.resolve(namespace, path, ref)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s/%s", ErrNotFound, namespace, path)
		}
		return err
	}
	pruneEmptyDirs(filepath.Dir(full), base)
	return nil
}

// pruneEmptyDirs removes empty directories walking up from dir until it reaches
// stopAt (exclusive) or hits a non-empty directory.
func pruneEmptyDirs(dir, stopAt string) {
	for dir != stopAt && strings.HasPrefix(dir, stopAt) {
		if err := os.Remove(dir); err != nil {
			return // non-empty or gone; stop pruning
		}
		dir = filepath.Dir(dir)
	}
}

// Match is a single search hit.
type Match struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"` // namespace-relative, slash-separated
	AbsPath   string `json:"abs_path"`
	Line      int    `json:"line"`
	Text      string `json:"text"`
}

// Search scans file contents for query (case-insensitive substring). When
// namespace is empty, all namespaces are searched. maxResults caps the number
// of hits (<= 0 means no cap).
func (s *Store) Search(namespace, query string, maxResults int, ref Ref) ([]Match, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is empty", ErrBadPath)
	}
	needle := strings.ToLower(query)

	var namespaces []string
	if namespace == "" {
		namespaces = Namespaces
	} else {
		namespaces = []string{namespace}
	}

	var matches []Match
	for _, ns := range namespaces {
		base, err := s.NamespaceDir(ns, ref)
		if err != nil {
			// Skip repo/branch namespaces when outside a git repo during an
			// all-namespace search; surface the error for a targeted search.
			if namespace == "" && (errors.Is(err, ErrNotInRepo)) {
				continue
			}
			return nil, err
		}
		if err := searchDir(ns, base, needle, maxResults, &matches); err != nil {
			return nil, err
		}
		if maxResults > 0 && len(matches) >= maxResults {
			return matches[:maxResults], nil
		}
	}
	return matches, nil
}

func searchDir(namespace, base, needle string, maxResults int, out *[]Match) error {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if maxResults > 0 && len(*out) >= maxResults {
			return filepath.SkipAll
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		rel, _ := filepath.Rel(base, path)
		relSlash := filepath.ToSlash(rel)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), needle) {
				*out = append(*out, Match{
					Namespace: namespace,
					Path:      relSlash,
					AbsPath:   path,
					Line:      lineNo,
					Text:      strings.TrimSpace(line),
				})
				if maxResults > 0 && len(*out) >= maxResults {
					return filepath.SkipAll
				}
			}
		}
		return scanner.Err()
	})
}
