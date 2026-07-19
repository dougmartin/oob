package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dougmartin/oob/internal/setup"
	"github.com/dougmartin/oob/internal/store"
)

func newInitCmd() *cobra.Command {
	var (
		force        bool
		claudeMdPath string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the oob directories and register the MCP server with Claude Code",
		Long: `Initialize oob on this machine:

  1. Create the ~/oob/{global,repos,branches} directories if they are missing.
  2. Register the 'oob' MCP server with Claude Code at user scope.
  3. Add an oob section to the global CLAUDE.md so Claude knows to use it.

Safe to run repeatedly; existing state is left in place unless --force is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			// 1. Directory layout.
			s, err := store.New()
			if err != nil {
				return err
			}
			if err := s.EnsureLayout(); err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ oob home ready at %s\n", s.Home)
			fmt.Fprintf(out, "    %s\n    %s\n    %s\n", s.GlobalDir(), s.ReposDir(), s.BranchesDir())

			// 2. MCP registration.
			bin, err := setup.BinaryPath()
			if err != nil {
				return err
			}
			res, err := setup.RegisterMCP(bin, force)
			if err != nil {
				return err
			}
			switch {
			case res.AlreadyPresent:
				fmt.Fprintln(out, "✓ oob MCP server already registered (use --force to re-register)")
			case res.Registered:
				fmt.Fprintf(out, "✓ registered oob MCP server -> %s serve\n", bin)
			}

			// 3. CLAUDE.md guidance.
			mdPath := claudeMdPath
			if mdPath == "" {
				if mdPath, err = setup.DefaultClaudeMdPath(); err != nil {
					return err
				}
			}
			action, err := setup.UpdateClaudeMd(mdPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "✓ CLAUDE.md %s (%s)\n", action, mdPath)

			fmt.Fprintln(out, "\nDone. Restart Claude Code (or reconnect MCP) to pick up the oob server.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "re-register the MCP server even if it already exists")
	cmd.Flags().StringVar(&claudeMdPath, "claude-md", "", "path to the CLAUDE.md to update (default ~/.claude/CLAUDE.md)")
	return cmd
}
