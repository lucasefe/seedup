package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	databaseURL   string
	migrationsDir string
	verbose       bool
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "seedup",
		Short:        "Database migration and seed management tool",
		SilenceUsage: true,
		Long: `seedup is a CLI tool for managing database migrations and seed data.
It wraps goose for migrations and provides utilities for creating and applying seed data.

Configuration is done via environment variables or CLI flags:
  DATABASE_URL    - PostgreSQL connection URL
  MIGRATIONS_DIR  - Path to migrations directory (default: ./migrations)
  SEED_DIR        - Path to seed data directory (default: ./seed)`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&databaseURL, "database-url", "d", "",
		"Database URL (or DATABASE_URL env)")
	rootCmd.PersistentFlags().StringVarP(&migrationsDir, "migrations-dir", "m", "",
		"Migrations directory (or MIGRATIONS_DIR env)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"Verbose output")

	// Add subcommands
	rootCmd.AddCommand(newMigrateCmd())
	rootCmd.AddCommand(newSeedCmd())
	rootCmd.AddCommand(newFlattenCmd())
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newDBCmd())
	rootCmd.AddCommand(newDBMLCmd())

	return rootCmd
}

// getDatabaseURL returns the database URL from flag or environment
func getDatabaseURL() string {
	if databaseURL != "" {
		return databaseURL
	}
	return os.Getenv("DATABASE_URL")
}

// getMigrationsDir returns the migrations directory from flag or environment
func getMigrationsDir() string {
	if migrationsDir != "" {
		return migrationsDir
	}
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		return dir
	}
	return "./migrations"
}

// getSeedDir returns the seed directory from flag or environment
func getSeedDir() string {
	if dir := os.Getenv("SEED_DIR"); dir != "" {
		return dir
	}
	return "./seed"
}
