// Package setup implements the one-time host configuration performed by
// `oob init`: creating the store layout, registering the MCP server with
// Claude Code, and documenting the server in the global CLAUDE.md.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// claudeMdStart and claudeMdEnd delimit the oob-managed block in CLAUDE.md so
// it can be updated idempotently.
const (
	claudeMdStart = "<!-- oob:start -->"
	claudeMdEnd   = "<!-- oob:end -->"
)

// claudeMdBody is the guidance written between the markers.
const claudeMdBody = `## Out-of-band (oob) files

An MCP server named ` + "`oob`" + ` manages **out-of-band files** — cross-repo and
cross-branch scratch/temp files that must NOT be committed into any git repo.
Whenever I mention an "oob file" (or "out of band" file), use the ` + "`oob`" + ` MCP
tools (` + "`oob_list`, `oob_read`, `oob_write`, `oob_delete`, `oob_search`, `oob_info`, `oob_path`" + `)
rather than writing into the working tree.

There are three namespaces:

- **global** — shared across every repo and branch. "Create a global oob file to…"
- **repo** — scoped to the current git repository. "Create a repo oob file that…"
- **branch** — scoped to the current repository *and* branch. "Create a branch oob file for…"

The repo and branch namespaces are resolved automatically from the current
working directory, so you do not need to name the repo or branch.

oob files are **plain files on disk**, not opaque blobs behind the MCP, and the
server is a thin filesystem wrapper (no index, no locking, ` + "`oob_search`" + ` scans
live). So for a **targeted edit of an existing file, use the normal Read/Edit
tools** on its absolute path rather than re-writing the whole file through
` + "`oob_write`" + `. Get that path from ` + "`oob_path`" + ` (it resolves files that don't exist
yet, too) or from the ` + "`abs_path`" + ` already returned by ` + "`oob_read`" + `, ` + "`oob_write`" + ` and
` + "`oob_search`" + `. Never hardcode oob paths. Use ` + "`oob_write`" + ` to create a file or
replace one wholesale.`

// BinaryPath returns the absolute, symlink-resolved path to the running oob
// binary, suitable for registering as a durable MCP command.
func BinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating oob binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}

// DefaultClaudeMdPath returns ~/.claude/CLAUDE.md.
func DefaultClaudeMdPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "CLAUDE.md"), nil
}

// MCPResult reports what RegisterMCP did.
type MCPResult struct {
	AlreadyPresent bool
	Registered     bool
	Output         string
}

// RegisterMCP registers the oob server with Claude Code at user scope by
// shelling out to `claude mcp add`. When the server is already registered it is
// left untouched unless force is set, in which case it is removed and re-added.
func RegisterMCP(binaryPath string, force bool) (MCPResult, error) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		return MCPResult{}, fmt.Errorf("the `claude` CLI was not found on PATH; install Claude Code or register the MCP manually: claude mcp add -s user oob -- %s serve", binaryPath)
	}

	present, err := mcpPresent(claude)
	if err != nil {
		return MCPResult{}, err
	}
	if present && !force {
		return MCPResult{AlreadyPresent: true}, nil
	}
	if present && force {
		// Ignore errors from remove; the add below is what matters.
		_ = exec.Command(claude, "mcp", "remove", "-s", "user", "oob").Run()
	}

	out, err := exec.Command(claude, "mcp", "add", "-s", "user", "oob", "--", binaryPath, "serve").CombinedOutput()
	if err != nil {
		return MCPResult{Output: string(out)}, fmt.Errorf("registering oob MCP server: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return MCPResult{Registered: true, Output: strings.TrimSpace(string(out))}, nil
}

func mcpPresent(claude string) (bool, error) {
	// `claude mcp get oob` exits non-zero when the server is absent.
	if err := exec.Command(claude, "mcp", "get", "oob").Run(); err == nil {
		return true, nil
	}
	// Fall back to scanning the list in case `get` is unavailable.
	out, err := exec.Command(claude, "mcp", "list").CombinedOutput()
	if err != nil {
		return false, nil // treat as absent; add will surface real problems
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "oob:") || strings.HasPrefix(trimmed, "oob ") {
			return true, nil
		}
	}
	return false, nil
}

// UpdateClaudeMd ensures the oob guidance block is present in the CLAUDE.md at
// path, creating the file (and parent directory) if needed. It returns the
// action taken: "created", "updated", or "unchanged".
func UpdateClaudeMd(path string) (action string, err error) {
	block := claudeMdStart + "\n" + claudeMdBody + "\n" + claudeMdEnd

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return "", readErr
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(block+"\n"), 0o644); err != nil {
			return "", err
		}
		return "created", nil
	}

	content := string(data)
	start := strings.Index(content, claudeMdStart)
	end := strings.Index(content, claudeMdEnd)
	if start != -1 && end != -1 && end > start {
		existing := content[start : end+len(claudeMdEnd)]
		if existing == block {
			return "unchanged", nil
		}
		updated := content[:start] + block + content[end+len(claudeMdEnd):]
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return "", err
		}
		return "updated", nil
	}

	// Append, ensuring separation from existing content.
	sep := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		sep = "\n\n"
	} else if len(content) > 0 && !strings.HasSuffix(content, "\n\n") {
		sep = "\n"
	}
	updated := content + sep + block + "\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return "updated", nil
}
