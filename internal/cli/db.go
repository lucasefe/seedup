package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/lucasefe/seedup/pkg/db"
)

var adminURL string

func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database setup and management commands",
		Long:  "Commands for managing database lifecycle: create, drop, and full setup",
	}

	cmd.PersistentFlags().StringVar(&adminURL, "admin-url", "",
		"Admin database URL for superuser operations (default: current system user)")

	cmd.AddCommand(newDBDropCmd())
	cmd.AddCommand(newDBCreateCmd())
	cmd.AddCommand(newDBSetupCmd())

	return cmd
}

func newDBDropCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "drop",
		Short: "Drop the database",
		Long:  "Drop the database specified in DATABASE_URL. This is a destructive operation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			cfg, err := db.ParseDatabaseURL(dbURL)
			if err != nil {
				return err
			}

			if !force {
				if !confirmAction(fmt.Sprintf("Drop database '%s'?", cfg.Database)) {
					fmt.Println("Aborted.")
					return nil
				}
			}

			m := db.New()

			if err := m.Drop(context.Background(), dbURL, adminURL); err != nil {
				return err
			}

			fmt.Printf("Database '%s' dropped successfully.\n", cfg.Database)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func newDBCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create the database",
		Long:  "Create the database specified in DATABASE_URL if it does not exist.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			cfg, err := db.ParseDatabaseURL(dbURL)
			if err != nil {
				return err
			}

			m := db.New()

			if err := m.Create(context.Background(), dbURL, adminURL); err != nil {
				return err
			}

			fmt.Printf("Database '%s' created successfully.\n", cfg.Database)
			return nil
		},
	}
}

func newDBSetupCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create database with user and permissions",
		Long: `Database setup: create user, drop (if exists), create database, set permissions.

This command only sets up the database infrastructure. To apply migrations
and seed data, run them separately:

  seedup db setup           # Create the database
  seedup seed apply dev     # Apply seed (runs initial migration + loads seed data)
  seedup migrate up         # Run remaining migrations

This is a destructive operation that will drop and recreate the database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			cfg, err := db.ParseDatabaseURL(dbURL)
			if err != nil {
				return err
			}

			if !force {
				if !confirmAction(fmt.Sprintf("This will DROP and recreate database '%s'. Continue?", cfg.Database)) {
					fmt.Println("Aborted.")
					return nil
				}
			}

			m := db.New()

			opts := db.SetupOptions{
				DatabaseURL: dbURL,
				AdminURL:    adminURL,
			}

			if err := m.Setup(context.Background(), opts); err != nil {
				return err
			}

			fmt.Println("Database setup completed successfully.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func confirmAction(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
