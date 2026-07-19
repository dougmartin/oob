// Package mcpserver exposes the oob store as MCP tools over stdio.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dougmartin/oob/internal/store"
)

// Version is the reported server/tool version.
const Version = "0.1.0"

// New builds an MCP server backed by the given store, with all oob tools
// registered.
func New(s *store.Store) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "oob",
		Version: Version,
		Title:   "Out-of-band file store",
	}, nil)
	register(srv, s)
	return srv
}

// Serve builds the server and runs it over stdio until the client disconnects
// or ctx is cancelled.
func Serve(ctx context.Context, s *store.Store) error {
	return New(s).Run(ctx, &mcp.StdioTransport{})
}

const namespaceDoc = "Namespace: 'global' (shared across all repos), 'repo' (the current git repository), or 'branch' (the current repo + branch). repo and branch are resolved from the server's working directory."

type listInput struct {
	Namespace string `json:"namespace" jsonschema:"the namespace to list; one of global, repo, branch"`
	Subpath   string `json:"subpath,omitempty" jsonschema:"optional subdirectory (relative to the namespace) to scope the listing"`
}

type listOutput struct {
	Namespace string        `json:"namespace"`
	Dir       string        `json:"dir"`
	Files     []store.Entry `json:"files"`
}

type readInput struct {
	Namespace string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path      string `json:"path" jsonschema:"file path relative to the namespace"`
}

type readOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	Content   string `json:"content"`
}

type writeInput struct {
	Namespace  string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path       string `json:"path" jsonschema:"file path relative to the namespace; parent directories are created as needed"`
	Content    string `json:"content" jsonschema:"the full file contents to write"`
	CreateOnly bool   `json:"create_only,omitempty" jsonschema:"when true, fail if the file already exists instead of overwriting"`
}

type writeOutput struct {
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	Created   bool   `json:"created"` // true if newly created, false if overwritten
}

type deleteInput struct {
	Namespace string `json:"namespace" jsonschema:"one of global, repo, branch"`
	Path      string `json:"path" jsonschema:"file path relative to the namespace"`
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
		Description: "List out-of-band (oob) files in a namespace, recursively. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listInput) (*mcp.CallToolResult, listOutput, error) {
		dir, err := s.NamespaceDir(in.Namespace)
		if err != nil {
			return toolErr[listOutput](err)
		}
		files, err := s.List(in.Namespace, in.Subpath)
		if err != nil {
			return toolErr[listOutput](err)
		}
		out := listOutput{Namespace: in.Namespace, Dir: dir, Files: files}
		var b strings.Builder
		fmt.Fprintf(&b, "%d file(s) in %s (%s)\n", len(files), in.Namespace, dir)
		for _, f := range files {
			fmt.Fprintf(&b, "  %s (%d bytes)\n", f.Path, f.Size)
		}
		return text(b.String()), out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_read",
		Description: "Read the contents of an oob file. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readInput) (*mcp.CallToolResult, readOutput, error) {
		content, err := s.Read(in.Namespace, in.Path)
		if err != nil {
			return toolErr[readOutput](err)
		}
		return text(content), readOutput{Namespace: in.Namespace, Path: in.Path, Content: content}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_write",
		Description: "Create or overwrite an oob file with the given contents (upsert). Set create_only to fail if it already exists. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in writeInput) (*mcp.CallToolResult, writeOutput, error) {
		created, err := s.Write(in.Namespace, in.Path, in.Content, in.CreateOnly)
		if err != nil {
			return toolErr[writeOutput](err)
		}
		verb := "updated"
		if created {
			verb = "created"
		}
		return text(fmt.Sprintf("%s %s/%s", verb, in.Namespace, in.Path)),
			writeOutput{Namespace: in.Namespace, Path: in.Path, Created: created}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_delete",
		Description: "Delete an oob file. Empty parent directories are pruned. " + namespaceDoc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteInput) (*mcp.CallToolResult, deleteOutput, error) {
		if err := s.Delete(in.Namespace, in.Path); err != nil {
			return toolErr[deleteOutput](err)
		}
		return text(fmt.Sprintf("deleted %s/%s", in.Namespace, in.Path)),
			deleteOutput{Namespace: in.Namespace, Path: in.Path, Deleted: true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "oob_search",
		Description: "Search oob file contents for a case-insensitive substring across one or all namespaces. Returns matching file paths, line numbers, and line text.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		max := in.MaxResults
		if max == 0 {
			max = 100
		}
		matches, err := s.Search(in.Namespace, in.Query, max)
		if err != nil {
			return toolErr[searchOutput](err)
		}
		out := searchOutput{Query: in.Query, Matches: matches, Count: len(matches)}
		var b strings.Builder
		fmt.Fprintf(&b, "%d match(es) for %q\n", len(matches), in.Query)
		for _, m := range matches {
			fmt.Fprintf(&b, "  %s/%s:%d: %s\n", m.Namespace, m.Path, m.Line, m.Text)
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
