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
//   - [DBSetup] - Full database setup (drop, create, migrate, seed)
//
// # Utility Functions
//
//   - [Flatten] - Flatten all migrations into a single initial migration
//   - [Check] - Validate migration timestamps (for CI)
//   - [GenerateDBML] - Generate DBML schema documentation
package seedup

import (
	"context"

	"github.com/tmwinc/seedup/pkg/check"
	"github.com/tmwinc/seedup/pkg/db"
	"github.com/tmwinc/seedup/pkg/dbml"
	"github.com/tmwinc/seedup/pkg/executor"
	"github.com/tmwinc/seedup/pkg/migrate"
	"github.com/tmwinc/seedup/pkg/seed"
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
}

// DBOptions configures database operations.
type DBOptions struct {
	// AdminURL is the connection URL for admin operations.
	// If empty, defaults to the current system user connecting to the postgres database.
	AdminURL string
}

// DBSetupOptions configures the full database setup operation.
type DBSetupOptions struct {
	// DatabaseURL is the target database connection URL.
	DatabaseURL string

	// AdminURL is the connection URL for admin operations.
	// If empty, defaults to the current system user connecting to the postgres database.
	AdminURL string

	// MigrationsDir is the path to the migrations directory.
	MigrationsDir string

	// SeedDir is the root seed directory (e.g., "./seed").
	SeedDir string

	// SeedName is the name of the seed set to apply (e.g., "dev" -> ./seed/dev/).
	SeedName string

	// SkipSeed skips applying seed data when true.
	SkipSeed bool
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

// SeedApply applies seed data from CSV files to the database.
// It runs the initial migration, loads seed data, then runs remaining migrations.
//
// The seedDir should contain CSV files named after the tables they populate.
//
// Example:
//
//	err := seedup.SeedApply(ctx, dbURL, "./migrations", "./seed/dev")
func SeedApply(ctx context.Context, dbURL, migrationsDir, seedDir string) error {
	exec := executor.New()
	s := seed.New(exec)
	return s.Apply(ctx, dbURL, migrationsDir, seedDir)
}

// SeedCreate creates seed data from an existing database.
// It reads a query file, executes it, and exports results to CSV files.
//
// The queryFile should contain SQL that populates temporary tables.
// Results are saved to seedDir as CSV files.
//
// Example:
//
//	err := seedup.SeedCreate(ctx, dbURL, "./migrations", "./seed/dev", "./seed/dev.sql", seedup.SeedCreateOptions{})
func SeedCreate(ctx context.Context, dbURL, migrationsDir, seedDir, queryFile string, opts SeedCreateOptions) error {
	exec := executor.New()
	s := seed.New(exec)
	return s.Create(ctx, dbURL, migrationsDir, seedDir, queryFile, seed.CreateOptions{
		DryRun: opts.DryRun,
	})
}

// DBCreate creates the database specified in the connection URL.
// Does nothing if the database already exists.
//
// Example:
//
//	err := seedup.DBCreate(ctx, "postgres://user:pass@localhost/mydb", seedup.DBOptions{})
func DBCreate(ctx context.Context, dbURL string, opts DBOptions) error {
	exec := executor.New()
	m := db.New(exec)
	return m.Create(ctx, dbURL, opts.AdminURL)
}

// DBDrop drops the database specified in the connection URL.
// Does nothing if the database doesn't exist.
//
// Example:
//
//	err := seedup.DBDrop(ctx, "postgres://user:pass@localhost/mydb", seedup.DBOptions{})
func DBDrop(ctx context.Context, dbURL string, opts DBOptions) error {
	exec := executor.New()
	m := db.New(exec)
	return m.Drop(ctx, dbURL, opts.AdminURL)
}

// DBSetup performs a full database setup: drop, create user, create db, set permissions,
// run migrations, and optionally apply seeds.
//
// This is a destructive operation that will drop and recreate the database.
//
// Example:
//
//	err := seedup.DBSetup(ctx, seedup.DBSetupOptions{
//	    DatabaseURL:   "postgres://user:pass@localhost/mydb",
//	    MigrationsDir: "./migrations",
//	    SeedDir:       "./seed",
//	    SeedName:      "dev",
//	})
func DBSetup(ctx context.Context, opts DBSetupOptions) error {
	exec := executor.New()
	m := db.New(exec)
	return m.Setup(ctx, db.SetupOptions{
		DatabaseURL:   opts.DatabaseURL,
		AdminURL:      opts.AdminURL,
		MigrationsDir: opts.MigrationsDir,
		SeedDir:       opts.SeedDir,
		SeedName:      opts.SeedName,
		SkipSeed:      opts.SkipSeed,
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
	exec := executor.New()
	f := migrate.NewFlattener(exec)
	return f.Flatten(ctx, dbURL, migrationsDir)
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
