package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/lucasefe/seedup/pkg/seed"
)

var (
	dryRun bool
)

func newSeedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Seed data management commands",
		Long:  "Commands for creating and applying database seed data",
	}

	cmd.AddCommand(newSeedApplyCmd())
	cmd.AddCommand(newSeedCreateCmd())

	return cmd
}

func newSeedApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Apply seed data to the database",
		Long: `Apply seed data to the database.

The <name> argument specifies which seed set to apply (e.g., "dev", "staging").
This loads seed/<name>/load.sql, runs migrations, and imports the data.

Example:
  seedup seed apply dev -d postgres://user:pass@localhost/mydb`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			s := seed.New()

			// Seed data directory: ./seed/<name>/
			dir := filepath.Join(getSeedDir(), name)

			return s.Apply(context.Background(), dbURL, getMigrationsDir(), dir)
		},
	}

	return cmd
}

func newSeedCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create seed data from a database",
		Long: `Create seed data from a database.

The <name> argument specifies the seed set name (e.g., "dev", "staging").
This reads the query file at seed/<name>/dump.sql, executes it against the database,
and exports the results to seed/<name>/load.sql.

The seed query file should contain SQL that populates temporary tables with the
data you want to include in the seed. Each table in the database has a corresponding
temp table named "seed.<schema>.<table>" that you should INSERT INTO.

Example:
  seedup seed create dev -d postgres://user:pass@localhost/production_db`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			s := seed.New()

			// Seed data directory: ./seed/<name>/
			dir := filepath.Join(getSeedDir(), name)

			// Seed query file: ./seed/<name>/dump.sql
			queryFile := filepath.Join(dir, "dump.sql")

			opts := seed.CreateOptions{
				DryRun: dryRun,
			}

			return s.Create(context.Background(), dbURL, dir, queryFile, opts)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without modifying files")

	return cmd
}
