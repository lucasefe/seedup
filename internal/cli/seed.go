package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/lucasefe/seedup/pkg/migrate"
	"github.com/lucasefe/seedup/pkg/pgconn"
	"github.com/lucasefe/seedup/pkg/seed"
	"github.com/spf13/cobra"
)

var (
	dryRun     bool
	schemas    string
	allSchemas bool
	noFlatten  bool
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
This runs the initial migration and loads seed/<name>/load.sql.

Run 'migrate up' separately after this command to apply remaining migrations.

Example:
  seedup seed apply dev -d postgres://user:pass@localhost/mydb
  seedup migrate up  # run remaining migrations`,
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
  seedup seed create dev -d postgres://user:pass@localhost/production_db
  seedup seed create dev -d postgres://... --all-schemas --exclude-extensions aiven_extras`,
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
				DryRun:     dryRun,
				Schemas:    splitList(schemas),
				AllSchemas: allSchemas,
			}

			if err := s.Create(context.Background(), dbURL, dir, queryFile, opts); err != nil {
				return err
			}

			// Run flatten after seed create unless --no-flatten is specified
			if !noFlatten && !dryRun {
				fmt.Println("[6/6] Flattening migrations...")
				db, err := pgconn.Open(dbURL)
				if err != nil {
					return fmt.Errorf("opening database for flatten: %w", err)
				}
				defer db.Close()

				f := migrate.NewFlattener(db, migrate.WithExcludeExtensions(getExcludeExtensions()))
				if err := f.Flatten(context.Background(), getMigrationsDir()); err != nil {
					return fmt.Errorf("flattening migrations: %w", err)
				}
				fmt.Println("      Migrations flattened successfully")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without modifying files")
	cmd.Flags().StringVar(&schemas, "schemas", "", "Comma-separated list of schemas to include (default: public)")
	cmd.Flags().BoolVarP(&allSchemas, "all-schemas", "a", false, "Include all non-system schemas")
	cmd.Flags().BoolVar(&noFlatten, "no-flatten", false, "Skip flattening migrations after seed creation")
	addExcludeExtensionsFlag(cmd)

	return cmd
}
