package cmd

import (
	"github.com/spf13/cobra"

	"github.com/dougmartin/oob/internal/mcpserver"
	"github.com/dougmartin/oob/internal/store"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the oob MCP server over stdio",
		Long: `Run the oob MCP server over stdio.

This is the command registered with Claude Code by 'oob init'. It exposes the
oob_list, oob_read, oob_write, oob_delete, oob_search and oob_info tools. The
'repo' and 'branch' namespaces are resolved from this process's working
directory, so run it from within the repository you want to target.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := store.New()
			if err != nil {
				return err
			}
			if err := s.EnsureLayout(); err != nil {
				return err
			}
			return mcpserver.Serve(cmd.Context(), s)
		},
	}
}
