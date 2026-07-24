// Package mcpserver exposes the oob store as MCP tools over stdio.
package mcpserver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dougmartin/oob/internal/store"
)

// Version is the reported server/tool version.
const Version = "0.1.0"

// instructions is sent to the client at initialize time. It exists mainly to
// stop agents from treating oob files as opaque blobs behind the MCP: they are
// ordinary files on disk, and targeted edits should use the client's own
// Read/Edit tools rather than a full-file oob_write.
const instructions = `oob stores out-of-band files: cross-repo and cross-branch scratch files kept outside any git working tree so they are never committed.

Namespaces:
  global -> <home>/global/<path>
  repo   -> <home>/repos/<repo>/<path>
  branch -> <home>/branches/<repo>/<branch>/<path>
<home> defaults to ~/oob (OOB_HOME overrides it). repo/branch are resolved from the git repository containing the server's working directory; oob_info reports the current values, and every file tool accepts optional repo/branch overrides. Do not hardcode these paths: branch names may contain slashes and the home may move, so call oob_path (or reuse the abs_path returned by oob_read, oob_write and oob_search) as the source of truth.

These are plain files on a local filesystem, not opaque blobs. This server is a thin wrapper over the filesystem: no index, no locking, no validation, and oob_search scans the files live. Editing a file directly is therefore equivalent to editing it through this server.

So prefer your own filesystem tools for targeted work:
  - Small or partial change to an existing file: get its abs_path, then use your normal Read/Edit tools on that path. Do NOT re-emit the whole file through oob_write for a one-line change.
  - Whole-file replacement, or creating a file (parent directories are created for you, and create_only guards against clobbering): use oob_write.
  - Discovery, and any client without filesystem access: use oob_list, oob_read, oob_search, oob_delete.`

// New builds an MCP server backed by the given store, with all oob tools
// registered.
func New(s *store.Store) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "oob",
		Version: Version,
		Title:   "Out-of-band file store",
	}, &mcp.ServerOptions{Instructions: instructions})
	register(srv, s)
	return srv
}

// Serve builds the server and runs it over stdio until the client disconnects
// or ctx is cancelled.
func Serve(ctx context.Context, s *store.Store) error {
	return New(s).Run(ctx, &mcp.StdioTransport{})
}

const namespaceDoc = "Namespace: 'global' (shared across all repos), 'repo' (the current git repository), or 'branch' (the current repo + branch). repo and branch default to the git repository of the server's working directory; override them with the 'repo'/'branch' arguments to target a different repository or branch."

// overrideRepoDoc and overrideBranchDoc document the optional ref overrides
// shared by the repo/branch-aware tools.
const (
	overrideRepoDoc   = "optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"
	overrideBranchDoc = "optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"
)

// editDoc is appended to the tools that hand back an abs_path, reminding
// callers that oob files are ordinary files they can edit in place.
const editDoc = "oob files are plain files on disk; the returned abs_path can be opened directly with your normal Read/Edit tools, which is the preferred way to make a targeted edit instead of re-writing the whole file."

// ref builds a store.Ref from the shared override fields.
func ref(repo, branch string) store.Ref { return store.Ref{Repo: repo, Branch: branch} }

type pathInput struct {
	Namespace string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path      string `json:"path" jsonschema:"file path relative to the namespace; it does not need to exist yet"`
	Repo      string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch    string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type pathOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	AbsPath   string `json:"abs_path"`
	Dir       string `json:"dir"`    // the namespace root abs_path is relative to
	Exists    bool   `json:"exists"` // whether a file is already there
}

type listInput struct {
	Namespace string `json:"namespace" jsonschema:"the namespace to list; one of global, repo, branch"`
	Subpath   string `json:"subpath,omitempty" jsonschema:"optional subdirectory (relative to the namespace) to scope the listing"`
	Repo      string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch    string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type listOutput struct {
	Namespace string        `json:"namespace"`
	Dir       string        `json:"dir"`
	Files     []store.Entry `json:"files"`
}

type readInput struct {
	Namespace string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path      string `json:"path" jsonschema:"file path relative to the namespace"`
	Repo      string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch    string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type readOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	AbsPath   string `json:"abs_path"`
	Content   string `json:"content"`
}

type writeInput struct {
	Namespace  string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path       string `json:"path" jsonschema:"file path relative to the namespace; parent directories are created as needed"`
	Content    string `json:"content" jsonschema:"the full file contents to write"`
	CreateOnly bool   `json:"create_only,omitempty" jsonschema:"when true, fail if the file already exists instead of overwriting"`
	Repo       string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch     string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type writeOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	AbsPath   string `json:"abs_path"`
	Created   bool   `json:"created"` // true if newly created, false if overwritten
}

type deleteInput struct {
	Namespace string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path      string `json:"path" jsonschema:"file path relative to the namespace"`
	Repo      string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch    string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type deleteOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	Deleted   bool   `json:"deleted"`
}

type searchInput struct {
	Query      string `json:"query" jsonschema:"case-insensitive substring to search for in file contents"`
	Namespace  string `json:"namespace,omitempty" jsonschema:"optional namespace to limit the search (global, repo, branch); omit to search all"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum number of matches to return (default 100)"`
	Repo       string `json:"repo,omitempty" jsonschema:"optional: repository name to target instead of the one detected from the working directory (applies to the repo and branch namespaces)"`
	Branch     string `json:"branch,omitempty" jsonschema:"optional: branch name to target instead of the current branch (applies to the branch namespace); may contain slashes"`
}

type searchOutput struct {
	Query   string        `json:"query"`
	Matches []store.Match `json:"matches"`
	Count   int           `json:"count"`
}

type infoOutput struct {
	*store.Context
}

func register(srv *mcp.Server, s *store.Store) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_info",
		Description: "Report the oob home directory and the git repo/branch resolved from the current working directory. Use this to see which repo and branch the 'repo' and 'branch' namespaces currently point at.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, infoOutput, error) {
		c, err := s.Context()
		if err != nil {
			return toolErr[infoOutput](err)
		}
		return text(fmt.Sprintf("home=%s in_git=%t repo=%s branch=%s", c.Home, c.InGit, c.RepoName, c.Branch)), infoOutput{c}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_list",
		Description: "List out-of-band (oob) files in a namespace, recursively. Returns the namespace directory as 'dir'; each file's absolute path is dir + '/' + its listed path. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
		r := ref(in.Repo, in.Branch)
		dir, err := s.NamespaceDir(in.Namespace, r)
		if err != nil {
			return toolErr[listOutput](err)
		}
		files, err := s.List(in.Namespace, in.Subpath, r)
		if err != nil {
			return toolErr[listOutput](err)
		}
		out := listOutput{Namespace: in.Namespace, Dir: dir, Files: files}
		var b strings.Builder
		fmt.Fprintf(&b, "%d file(s) in %s, under %s\n", len(files), in.Namespace, dir)
		for _, f := range files {
			fmt.Fprintf(&b, "  %s (%d bytes)\n", f.Path, f.Size)
		}
		return text(b.String()), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_path",
		Description: "Resolve the absolute filesystem path of an oob file without reading it. The file need not exist, so this also answers \"where should I create this?\". Use it instead of guessing paths from the namespace layout: it is the source of truth for the oob home, repo-name derivation, and branch names containing slashes. " + editDoc + " " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pathInput) (*mcp.CallToolResult, pathOutput, error) {
		r := ref(in.Repo, in.Branch)
		dir, err := s.NamespaceDir(in.Namespace, r)
		if err != nil {
			return toolErr[pathOutput](err)
		}
		abs, err := s.AbsPath(in.Namespace, in.Path, r)
		if err != nil {
			return toolErr[pathOutput](err)
		}
		exists := false
		if _, statErr := os.Stat(abs); statErr == nil {
			exists = true
		}
		return text(abs), pathOutput{
			Namespace: in.Namespace, Path: in.Path, AbsPath: abs, Dir: dir, Exists: exists,
		}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_read",
		Description: "Read the contents of an oob file. " + editDoc + " " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readInput) (*mcp.CallToolResult, readOutput, error) {
		r := ref(in.Repo, in.Branch)
		content, err := s.Read(in.Namespace, in.Path, r)
		if err != nil {
			return toolErr[readOutput](err)
		}
		abs, err := s.AbsPath(in.Namespace, in.Path, r)
		if err != nil {
			return toolErr[readOutput](err)
		}
		return text(content), readOutput{Namespace: in.Namespace, Path: in.Path, AbsPath: abs, Content: content}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_write",
		Description: "Create or overwrite an oob file with the given contents (upsert). Set create_only to fail if it already exists. Best for creating a file or replacing one wholesale; for a targeted change to an existing file, edit its abs_path in place with your normal Edit tool rather than re-emitting the whole file here. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in writeInput) (*mcp.CallToolResult, writeOutput, error) {
		r := ref(in.Repo, in.Branch)
		created, err := s.Write(in.Namespace, in.Path, in.Content, in.CreateOnly, r)
		if err != nil {
			return toolErr[writeOutput](err)
		}
		abs, err := s.AbsPath(in.Namespace, in.Path, r)
		if err != nil {
			return toolErr[writeOutput](err)
		}
		verb := "updated"
		if created {
			verb = "created"
		}
		return text(fmt.Sprintf("%s %s/%s (%s)", verb, in.Namespace, in.Path, abs)),
			writeOutput{Namespace: in.Namespace, Path: in.Path, AbsPath: abs, Created: created}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_delete",
		Description: "Delete an oob file. Empty parent directories are pruned. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteInput) (*mcp.CallToolResult, deleteOutput, error) {
		if err := s.Delete(in.Namespace, in.Path, ref(in.Repo, in.Branch)); err != nil {
			return toolErr[deleteOutput](err)
		}
		return text(fmt.Sprintf("deleted %s/%s", in.Namespace, in.Path)),
			deleteOutput{Namespace: in.Namespace, Path: in.Path, Deleted: true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_search",
		Description: "Search oob file contents for a case-insensitive substring across one or all namespaces. Returns matching file paths, absolute paths, line numbers, and line text. " + editDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		max := in.MaxResults
		if max == 0 {
			max = 100
		}
		matches, err := s.Search(in.Namespace, in.Query, max, ref(in.Repo, in.Branch))
		if err != nil {
			return toolErr[searchOutput](err)
		}
		out := searchOutput{Query: in.Query, Matches: matches, Count: len(matches)}
		var b strings.Builder
		fmt.Fprintf(&b, "%d match(es) for %q\n", len(matches), in.Query)
		for _, m := range matches {
			fmt.Fprintf(&b, "  [%s] %s:%d: %s\n", m.Namespace, m.AbsPath, m.Line, m.Text)
		}
		return text(b.String()), out, nil
	})
}

// text builds a CallToolResult carrying a single text content block.
func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// toolErr returns a tool-level (non-protocol) error result. The zero Out value
// is discarded by the SDK when an error is present.
func toolErr[Out any](err error) (*mcp.CallToolResult, Out, error) {
	var zero Out
	return nil, zero, err
}
