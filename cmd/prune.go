package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dougmartin/oob/internal/store"
)

func newPruneCmd() *cobra.Command {
	var doDelete bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove branch oob files for branches that no longer exist in the current repo",
		Long: `Reconcile the branch namespace against the current repository's live branches.

Run from inside a git repository. Any branch directory under
~/oob/branches/<repo>/ whose branch no longer exists (per 'git for-each-ref
refs/heads') is reported. By default this is a dry run; pass --delete to remove
the stale directories.

Only the current repository is pruned, since oob stores repos by name and needs
the live branch list from the actual git repo to reconcile safely.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			s, err := store.New()
			if err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			gc, err := store.DetectGit(wd)
			if err != nil {
				return fmt.Errorf("prune must be run inside a git repository: %w", err)
			}
			live, err := store.LiveBranches(wd)
			if err != nil {
				return fmt.Errorf("listing branches: %w", err)
			}

			stale, err := s.StaleBranches(gc.RepoName, live)
			if err != nil {
				return err
			}
			if len(stale) == 0 {
				fmt.Fprintf(out, "Nothing to prune: all branch oob files for %q map to live branches.\n", gc.RepoName)
				return nil
			}

			total := 0
			fmt.Fprintf(out, "Stale branch namespaces for %q:\n", gc.RepoName)
			for _, sb := range stale {
				fmt.Fprintf(out, "  %s  (%d file(s))\n", sb.Branch, sb.Files)
				total += sb.Files
			}

			if !doDelete {
				fmt.Fprintf(out, "\n%d directory(ies), %d file(s) would be removed. Re-run with --delete to remove them.\n", len(stale), total)
				return nil
			}

			for _, sb := range stale {
				if err := s.PruneBranch(gc.RepoName, sb.Branch); err != nil {
					return fmt.Errorf("removing %s: %w", sb.Branch, err)
				}
			}
			fmt.Fprintf(out, "\nRemoved %d directory(ies), %d file(s).\n", len(stale), total)
			return nil
		},
	}
	cmd.Flags().BoolVar(&doDelete, "delete", false, "delete the stale branch directories (default is a dry run)")
	return cmd
}
