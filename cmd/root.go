// Package cmd implements the oob command-line interface.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dougmartin/oob/internal/mcpserver"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "oob",
		Short: "Manage out-of-band (oob) files for git repos",
		Long: `oob is a CRUD system for out-of-band files kept outside your git repos.

Files live under ~/oob in three namespaces:
  global   shared across every repo and branch
  repo     scoped to the current git repository
  branch   scoped to the current repository + branch

The 'oob serve' command runs an MCP server exposing these files to Claude Code.`,
		Version:      mcpserver.Version,
		SilenceUsage: true,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newPruneCmd())
	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
