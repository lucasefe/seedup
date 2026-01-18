package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/lucasefe/seedup/pkg/migrate"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Database migration commands",
		Long:  "Commands for managing database migrations using goose",
	}

	cmd.AddCommand(newMigrateUpCmd())
	cmd.AddCommand(newMigrateUpByOneCmd())
	cmd.AddCommand(newMigrateDownCmd())
	cmd.AddCommand(newMigrateStatusCmd())
	cmd.AddCommand(newMigrateCreateCmd())

	return cmd
}

func newMigrateUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Run all pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			m := migrate.New(migrate.WithVerbose(verbose))
			return m.Up(context.Background(), dbURL, getMigrationsDir())
		},
	}
}

func newMigrateUpByOneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up-by-one",
		Short: "Run a single pending migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			m := migrate.New(migrate.WithVerbose(verbose))
			return m.UpByOne(context.Background(), dbURL, getMigrationsDir())
		},
	}
}

func newMigrateDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Rollback the last migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			m := migrate.New(migrate.WithVerbose(verbose))
			return m.Down(context.Background(), dbURL, getMigrationsDir())
		},
	}
}

func newMigrateStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			m := migrate.New(migrate.WithVerbose(verbose))
			return m.Status(context.Background(), dbURL, getMigrationsDir())
		},
	}
}

func newMigrateCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new migration file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m := migrate.New(migrate.WithVerbose(verbose))

			filepath, err := m.Create(getMigrationsDir(), args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stdout, "Created migration: %s\n", filepath)
			return nil
		},
	}
}
