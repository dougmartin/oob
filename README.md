# oob — out-of-band files for git repos

`oob` is a small Go CLI **and** MCP server for managing *out-of-band* files:
cross-repo and cross-branch scratch/temp files you deliberately keep **outside**
your working tree so they never get committed.

Files live under `~/oob` in three namespaces:

| Namespace | Location                              | Scope                                   |
|-----------|---------------------------------------|-----------------------------------------|
| `global`  | `~/oob/global/…`                      | shared across every repo and branch     |
| `repo`    | `~/oob/repos/<repo>/…`                | the current git repository              |
| `branch`  | `~/oob/branches/<repo>/<branch>/…`    | the current repository **and** branch   |

`<repo>` is the basename of the git repository root; `<branch>` mirrors git's
own layout, so `feature/foo` nests as `feature/foo/`. The `repo` and `branch`
namespaces are resolved from the working directory of the running server — no
need to name the repo or branch.

## Install

Requires Go (managed here via `asdf`; see `.tool-versions`).

```bash
./install.sh          # build + install to ~/.local/bin
./install.sh --init   # ...and also run `oob init`
```

`install.sh` builds the binary, installs it to `~/.local/bin` (override with
`INSTALL_DIR=...`), and warns if that directory isn't on your PATH. Pass
`--init` to register the MCP server in the same step (see [`oob init`](#oob-init)).

Equivalently, via `make` or by hand:

```bash
make install          # builds to ~/.local/bin/oob and runs `oob init`
# or manually:
go build -o ~/.local/bin/oob .
oob init
```

## `oob init`

Run once per machine. It:

1. Creates `~/oob/{global,repos,branches}` if missing.
2. Registers the `oob` MCP server with Claude Code at **user** scope
   (`claude mcp add -s user oob -- <path-to-oob> serve`).
3. Adds an `oob` section to your global `~/.claude/CLAUDE.md` so Claude knows to
   route "oob file" requests through this server.

It is safe to re-run; existing state is left in place. Use `--force` to
re-register the MCP server, and `--claude-md <path>` to target a different
CLAUDE.md.

Set `OOB_HOME` to override the base directory (default `~/oob`).

## Usage from Claude Code

Once initialized, just ask in natural language:

- "Create a **global** oob file to track my scratch API keys layout."
- "Create a **repo** oob file that lists the flaky tests in this project."
- "Write a **branch** oob note with the migration steps for this feature."

Claude will use the MCP tools below instead of writing into the repo.

## MCP tools

| Tool         | Purpose                                                        |
|--------------|---------------------------------------------------------------|
| `oob_info`   | Report the oob home and the resolved repo/branch.             |
| `oob_list`   | List files in a namespace (recursive).                        |
| `oob_read`   | Read a file's contents.                                       |
| `oob_write`  | Create or overwrite a file (`create_only` to fail if exists). |
| `oob_delete` | Delete a file; empty parent dirs are pruned.                  |
| `oob_search` | Case-insensitive content search across one or all namespaces. |

## `oob serve`

Runs the MCP server over stdio. This is what `oob init` registers; you normally
don't invoke it directly. It resolves `repo`/`branch` from its working
directory, so it must run from inside the target repository (Claude Code does
this for you).

## Development

```bash
make test    # go test ./...
make vet
make fmt
```

Layout:

- `main.go` — entry point
- `cmd/` — cobra commands (`root`, `init`, `serve`)
- `internal/store/` — namespace resolution, git detection, filesystem CRUD
- `internal/setup/` — `init` host configuration (dirs, MCP registration, CLAUDE.md)
- `internal/mcpserver/` — MCP tool definitions over the store
