package cli

import (
	"context"
	"fmt"

	"github.com/lucasefe/seedup/pkg/check"
	"github.com/lucasefe/seedup/pkg/executor"
	"github.com/spf13/cobra"
)

var baseBranch string

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate migration timestamps",
		Long: `Validate that new migrations have the latest timestamps.
This prevents merge conflicts when multiple developers add migrations concurrently.

This command is intended for use in CI pipelines.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !check.IsGitRepo() {
				return fmt.Errorf("not in a git repository")
			}

			exec := executor.New(executor.WithVerbose(verbose))
			c := check.New(exec)

			return c.Check(context.Background(), getMigrationsDir(), baseBranch)
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base-branch", "main", "Base branch for comparison")

	return cmd
}
