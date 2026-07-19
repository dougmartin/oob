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

### Targeting another repo or branch

By default the `repo` and `branch` namespaces resolve from the working directory
of the running server (one fixed location for the session — check it with
`oob_info`). When you're working across several repos in a single session, every
CRUD tool also accepts optional `repo` and `branch` arguments to override that:

- "Write a **branch** oob note for repo `billing-api` on branch `feature/PROJ-42`…"
- "List the **repo** oob files for `web-frontend`."

`repo` alone applies to both the `repo` and `branch` namespaces; `branch` applies
to the `branch` namespace. Whatever you don't override still falls back to the
detected git context, so within a single repo you never need to pass them.

## `oob prune`

Reconcile the `branch` namespace against a repository's live branches. Run it
from inside a git repo; any directory under `~/oob/branches/<repo>/` whose branch
no longer exists (per `git for-each-ref refs/heads`) is reported.

```bash
oob prune            # dry run: list stale branch directories
oob prune --delete   # remove them
```

It's a dry run by default, and only the current repository is pruned — oob stores
repos by name and needs the live branch list from the real git repo to reconcile
safely. Directories for a live branch, and shared parent dirs (e.g. `feature/`
when `feature/foo` still exists), are preserved.

## `oob serve`

Runs the MCP server over stdio. This is what `oob init` registers; you normally
don't invoke it directly. It resolves `repo`/`branch` from its working
directory, so it must run from inside the target repository (Claude Code does
this for you) — or pass the `repo`/`branch` tool arguments to target another.

## Development

```bash
make test    # go test ./...
make vet
make fmt
```

Layout:

- `main.go` — entry point
- `cmd/` — cobra commands (`root`, `init`, `serve`, `prune`)
- `internal/store/` — namespace resolution, git detection, filesystem CRUD, prune
- `internal/setup/` — `init` host configuration (dirs, MCP registration, CLAUDE.md)
- `internal/mcpserver/` — MCP tool definitions over the store
