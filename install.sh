#!/usr/bin/env bash
#
# install.sh — build the oob binary and install it onto your PATH.
#
# Usage:
#   ./install.sh            build and install to ~/.local/bin
#   ./install.sh --init     ...then also run `oob init` (registers the MCP
#                           server with Claude Code and updates CLAUDE.md)
#
# Override the destination with INSTALL_DIR:
#   INSTALL_DIR=/usr/local/bin ./install.sh
#
set -euo pipefail

# Run from the repo root regardless of where this is invoked from.
cd "$(dirname "$0")"

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY="oob"
RUN_INIT=false

for arg in "$@"; do
	case "$arg" in
		--init) RUN_INIT=true ;;
		-h|--help)
			sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*)
			echo "install.sh: unknown option: $arg" >&2
			exit 2
			;;
	esac
done

if ! command -v go >/dev/null 2>&1; then
	echo "error: 'go' not found on PATH. Install Go (see .tool-versions) and retry." >&2
	exit 1
fi

echo "Building ${BINARY} with $(go version | awk '{print $3}')..."
mkdir -p "$INSTALL_DIR"
go build -o "$INSTALL_DIR/$BINARY" .

TARGET="$INSTALL_DIR/$BINARY"
echo "Installed -> $TARGET"

# Warn if the install dir is not on PATH.
case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		echo
		echo "warning: $INSTALL_DIR is not on your PATH."
		echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, ...):"
		echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
		;;
esac

if [ "$RUN_INIT" = true ]; then
	echo
	echo "Running 'oob init'..."
	"$TARGET" init
else
	echo
	echo "Done. Next, run 'oob init' to register the MCP server with Claude Code:"
	echo "    $BINARY init"
fi
