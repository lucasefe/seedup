// Package seedup provides database migration, seeding, and schema documentation tools.
//
// seedup can be used as both a CLI tool and a Go library. This package exposes
// the public API for programmatic use.
//
// # Quick Start
//
// Run all pending migrations:
//
//	err := seedup.MigrateUp(ctx, "postgres://user:pass@localhost/mydb", "./migrations")
//
// Generate DBML documentation:
//
//	dbml, err := seedup.GenerateDBML(ctx, dbURL, seedup.DBMLOptions{})
//
// # Migration Functions
//
// The migration functions wrap the goose migration library:
//   - [MigrateUp] - Run all pending migrations
//   - [MigrateUpByOne] - Run a single pending migration
//   - [MigrateDown] - Rollback the last migration
//   - [MigrateStatus] - Show migration status
//   - [MigrateCreate] - Create a new migration file
//
// # Seed Functions
//
// Seed functions help manage test/dev data:
//   - [SeedApply] - Apply seed data to the database
//   - [SeedCreate] - Create seed data from an existing database
//
// # Database Functions
//
// Database management functions:
//   - [DBCreate] - Create the database
//   - [DBDrop] - Drop the database
//   - [DBSetup] - Database setup (drop, create user, create db, permissions)
//
// # Utility Functions
//
//   - [Flatten] - Flatten all migrations into a single initial migration
//   - [Check] - Validate migration timestamps (for CI)
//   - [GenerateDBML] - Generate DBML schema documentation
package seedup

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasefe/seedup/internal/cli"
	"github.com/lucasefe/seedup/pkg/check"
	"github.com/lucasefe/seedup/pkg/db"
	"github.com/lucasefe/seedup/pkg/dbml"
	"github.com/lucasefe/seedup/pkg/executor"
	"github.com/lucasefe/seedup/pkg/migrate"
	"github.com/lucasefe/seedup/pkg/pgconn"
	"github.com/lucasefe/seedup/pkg/seed"
)

// DBMLOptions configures DBML generation.
type DBMLOptions struct {
	// Schemas specifies which database schemas to include.
	// If empty, defaults to "public".
	Schemas []string

	// ExcludeTables specifies tables to exclude from the output.
	ExcludeTables []string

	// AllSchemas includes all non-system schemas when true.
	AllSchemas bool
}

// SeedCreateOptions configures seed creation.
type SeedCreateOptions struct {
	// DryRun previews the operation without making changes.
	DryRun bool
	// Schemas specifies which database schemas to include.
	// If empty and AllSchemas is false, defaults to "public".
	Schemas []string
	// AllSchemas includes all non-system schemas when true.
	AllSchemas bool
	// NoFlatten skips flattening migrations after seed creation.
	// By default, flatten is run automatically after creating seeds.
	NoFlatten bool
	// MigrationsDir specifies the migrations directory for flatten.
	// Required unless NoFlatten is true.
	MigrationsDir string
}

// DBOptions configures database operations.
type DBOptions struct {
	// AdminURL is the connection URL for admin operations.
	// If empty, defaults to the current system user connecting to the postgres database.
	AdminURL string
}

// DBSetupOptions configures the database setup operation.
// Setup only creates the database infrastructure (user, database, permissions).
// Use [SeedApply] and [MigrateUp] separately to apply seeds and migrations.
type DBSetupOptions struct {
	// DatabaseURL is the target database connection URL.
	DatabaseURL string

	// AdminURL is the connection URL for admin operations.
	// If empty, defaults to the current system user connecting to the postgres database.
	AdminURL string
}

// MigrateUp runs all pending migrations.
//
// Example:
//
//	err := seedup.MigrateUp(ctx, "postgres://user:pass@localhost/mydb", "./migrations")
func MigrateUp(ctx context.Context, dbURL, migrationsDir string) error {
	m := migrate.New()
	return m.Up(ctx, dbURL, migrationsDir)
}

// MigrateUpByOne runs a single pending migration.
//
// Example:
//
//	err := seedup.MigrateUpByOne(ctx, dbURL, "./migrations")
func MigrateUpByOne(ctx context.Context, dbURL, migrationsDir string) error {
	m := migrate.New()
	return m.UpByOne(ctx, dbURL, migrationsDir)
}

// MigrateDown rolls back the last applied migration.
//
// Example:
//
//	err := seedup.MigrateDown(ctx, dbURL, "./migrations")
func MigrateDown(ctx context.Context, dbURL, migrationsDir string) error {
	m := migrate.New()
	return m.Down(ctx, dbURL, migrationsDir)
}

// MigrateStatus displays the status of all migrations.
// Output is written to stdout.
//
// Example:
//
//	err := seedup.MigrateStatus(ctx, dbURL, "./migrations")
func MigrateStatus(ctx context.Context, dbURL, migrationsDir string) error {
	m := migrate.New()
	return m.Status(ctx, dbURL, migrationsDir)
}

// MigrateCreate creates a new migration file with the given name.
// Returns the path to the created file.
//
// Example:
//
//	path, err := seedup.MigrateCreate("./migrations", "add_users_table")
//	// path = "./migrations/20240115123456_add_users_table.sql"
func MigrateCreate(migrationsDir, name string) (string, error) {
	m := migrate.New()
	return m.Create(migrationsDir, name)
}

// GenerateDBML generates DBML (Database Markup Language) documentation from the database schema.
// Returns the DBML content as a string.
//
// DBML can be visualized at https://dbdiagram.io
//
// Example:
//
//	content, err := seedup.GenerateDBML(ctx, dbURL, seedup.DBMLOptions{
//	    ExcludeTables: []string{"goose_db_version"},
//	})
func GenerateDBML(ctx context.Context, dbURL string, opts DBMLOptions) (string, error) {
	g := dbml.New()
	return g.GenerateString(ctx, dbURL, dbml.Options{
		Schemas:       opts.Schemas,
		ExcludeTables: opts.ExcludeTables,
		AllSchemas:    opts.AllSchemas,
	})
}

// SeedApply applies seed data from a load.sql file to the database.
// It runs the initial migration (to establish the schema the seed was created against),
// then loads seed data. Run [MigrateUp] separately to apply remaining migrations.
//
// The seedDir should contain a load.sql file with batched INSERT statements.
//
// Example:
//
//	err := seedup.SeedApply(ctx, dbURL, "./migrations", "./seed/dev")
//	// Then run remaining migrations:
//	err = seedup.MigrateUp(ctx, dbURL, "./migrations")
func SeedApply(ctx context.Context, dbURL, migrationsDir, seedDir string) error {
	s := seed.New()
	return s.Apply(ctx, dbURL, migrationsDir, seedDir)
}

// SeedCreate creates seed data from an existing database.
// It reads a query file, executes it, and exports results to a single load.sql file.
// After creating the seed, it automatically flattens migrations unless NoFlatten is set.
//
// The queryFile (dump.sql) should contain SQL that populates temporary tables.
// Results are saved to seedDir/load.sql as batched INSERT statements.
//
// Example:
//
//	err := seedup.SeedCreate(ctx, dbURL, "./seed/dev", "./seed/dev/dump.sql", seedup.SeedCreateOptions{
//	    MigrationsDir: "./migrations",
//	})
func SeedCreate(ctx context.Context, dbURL, seedDir, queryFile string, opts SeedCreateOptions) error {
	s := seed.New()
	if err := s.Create(ctx, dbURL, seedDir, queryFile, seed.CreateOptions{
		DryRun:     opts.DryRun,
		Schemas:    opts.Schemas,
		AllSchemas: opts.AllSchemas,
	}); err != nil {
		return err
	}

	// Run flatten after seed create unless NoFlatten is specified
	if !opts.NoFlatten && !opts.DryRun {
		if opts.MigrationsDir == "" {
			return fmt.Errorf("MigrationsDir is required for flatten (set NoFlatten to skip)")
		}

		conn, err := pgconn.Open(dbURL)
		if err != nil {
			return fmt.Errorf("opening database for flatten: %w", err)
		}
		defer conn.Close()

		f := migrate.NewFlattener(conn)
		if err := f.Flatten(ctx, opts.MigrationsDir); err != nil {
			return fmt.Errorf("flattening migrations: %w", err)
		}
	}

	return nil
}

// DBCreate creates the database specified in the connection URL.
// Does nothing if the database already exists.
//
// Example:
//
//	err := seedup.DBCreate(ctx, "postgres://user:pass@localhost/mydb", seedup.DBOptions{})
func DBCreate(ctx context.Context, dbURL string, opts DBOptions) error {
	m := db.New()
	return m.Create(ctx, dbURL, opts.AdminURL)
}

// DBDrop drops the database specified in the connection URL.
// Does nothing if the database doesn't exist.
//
// Example:
//
//	err := seedup.DBDrop(ctx, "postgres://user:pass@localhost/mydb", seedup.DBOptions{})
func DBDrop(ctx context.Context, dbURL string, opts DBOptions) error {
	m := db.New()
	return m.Drop(ctx, dbURL, opts.AdminURL)
}

// DBSetup creates a database with user and permissions.
// It creates the user, drops and recreates the database, and sets up permissions.
//
// This does NOT run migrations or apply seeds - use [SeedApply] and [MigrateUp] separately.
//
// This is a destructive operation that will drop and recreate the database.
//
// Example:
//
//	// Create database infrastructure
//	err := seedup.DBSetup(ctx, seedup.DBSetupOptions{
//	    DatabaseURL: "postgres://user:pass@localhost/mydb",
//	})
//	// Apply seed (runs initial migration + loads seed data)
//	err = seedup.SeedApply(ctx, dbURL, "./migrations", "./seed/dev")
//	// Run remaining migrations
//	err = seedup.MigrateUp(ctx, dbURL, "./migrations")
func DBSetup(ctx context.Context, opts DBSetupOptions) error {
	m := db.New()
	return m.Setup(ctx, db.SetupOptions{
		DatabaseURL: opts.DatabaseURL,
		AdminURL:    opts.AdminURL,
	})
}

// Flatten consolidates all applied migrations into a single initial migration.
// It dumps the current schema and replaces all migration files with a single file.
//
// This is useful for:
//   - Reducing the number of migration files
//   - Creating a clean starting point
//   - Simplifying migration history
//
// Example:
//
//	err := seedup.Flatten(ctx, dbURL, "./migrations")
func Flatten(ctx context.Context, dbURL, migrationsDir string) error {
	conn, err := pgconn.Open(dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer conn.Close()

	f := migrate.NewFlattener(conn)
	return f.Flatten(ctx, migrationsDir)
}

// Check validates that new migrations have the latest timestamps.
// This prevents merge conflicts when multiple developers add migrations concurrently.
//
// Intended for use in CI pipelines to ensure migrations are properly rebased.
//
// Example:
//
//	err := seedup.Check(ctx, "./migrations", "main")
func Check(ctx context.Context, migrationsDir, baseBranch string) error {
	exec := executor.New()
	c := check.New(exec)
	return c.Check(ctx, migrationsDir, baseBranch)
}

// Run executes a seedup CLI command from a command string.
// Environment variables (DATABASE_URL, etc.) are read from os.Getenv.
//
// Example:
//
//	err := seedup.Run("migrate up -d postgres://localhost/mydb")
func Run(command string) error {
	args := strings.Fields(command)
	return RunArgs(args...)
}

// RunArgs executes a seedup CLI command with pre-split arguments.
// This is useful for embedding seedup in other binaries.
// Environment variables (DATABASE_URL, etc.) are read from os.Getenv.
//
// Example:
//
//	// otherbinary seedup seed create dev
//	err := seedup.RunArgs(os.Args[2:]...)
func RunArgs(args ...string) error {
	cmd := cli.NewRootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}
