// Package main demonstrates how to use seedup as a Go library.
//
// This example shows all the main features of seedup:
//   - Running migrations
//   - Generating DBML documentation
//   - Creating migrations programmatically
//   - Flattening migrations into a single file
//
// To run this example:
//
//	cd examples
//	createdb seedup_example  # Create the database first
//	go run usage.go
//
// Prerequisites:
//   - PostgreSQL running locally
//   - Database 'seedup_example' created (createdb seedup_example)
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"

	_ "github.com/lib/pq"
	"github.com/tmwinc/seedup"
)

func main() {
	// Configuration - use current OS user (typical for local Homebrew PostgreSQL)
	// Uses EXAMPLE_DATABASE_URL to avoid conflicts with existing DATABASE_URL
	dbURL := getEnv("EXAMPLE_DATABASE_URL", defaultDatabaseURL())

	// Get the directory where this example lives
	exampleDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	migrationsDir := filepath.Join(exampleDir, "migrations")
	seedDir := filepath.Join(exampleDir, "seed")

	ctx := context.Background()

	fmt.Println("=== seedup Library Example ===")
	fmt.Printf("Using database: %s\n", dbURL)
	fmt.Println()

	// =========================================================================
	// 1. Reset Database Schema (drop all tables for clean slate)
	// =========================================================================
	fmt.Println("1. Resetting database schema...")

	if err := resetSchema(dbURL); err != nil {
		log.Fatalf("Failed to reset schema: %v", err)
	}
	fmt.Println("   Schema reset complete")
	fmt.Println()

	// =========================================================================
	// 2. Run Migrations
	// =========================================================================
	fmt.Println("2. Running migrations...")

	if err := seedup.MigrateUp(ctx, dbURL, migrationsDir); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	fmt.Println("   Migrations complete")
	fmt.Println()

	// =========================================================================
	// 3. Check Migration Status
	// =========================================================================
	fmt.Println("3. Migration status:")
	if err := seedup.MigrateStatus(ctx, dbURL, migrationsDir); err != nil {
		log.Fatalf("Failed to get migration status: %v", err)
	}
	fmt.Println()

	// =========================================================================
	// 4. Generate DBML Documentation
	// =========================================================================
	fmt.Println("4. Generating DBML documentation...")

	dbml, err := seedup.GenerateDBML(ctx, dbURL, seedup.DBMLOptions{
		ExcludeTables: []string{"goose_db_version"},
	})
	if err != nil {
		log.Fatalf("Failed to generate DBML: %v", err)
	}

	fmt.Println("   DBML output:")
	fmt.Println("   ---")
	fmt.Println(dbml)
	fmt.Println("   ---")
	fmt.Println()

	// =========================================================================
	// 5. Seed Data Info
	// =========================================================================
	fmt.Println("5. Seed data operations...")

	seedQueryFile := filepath.Join(seedDir, "dev.sql")
	seedOutputDir := filepath.Join(seedDir, "dev")

	fmt.Printf("   Seed query file: %s\n", seedQueryFile)
	fmt.Printf("   Seed output dir: %s\n", seedOutputDir)
	fmt.Println()
	fmt.Println("   To create seeds: seedup.SeedCreate(ctx, dbURL, migrationsDir, seedOutputDir, seedQueryFile, opts)")
	fmt.Println("   To apply seeds:  seedup.SeedApply(ctx, dbURL, migrationsDir, seedOutputDir)")
	fmt.Println()

	// =========================================================================
	// 6. Create a New Migration
	// =========================================================================
	fmt.Println("6. Creating a new migration...")

	newMigration, err := seedup.MigrateCreate(migrationsDir, "add_comments")
	if err != nil {
		log.Fatalf("Failed to create migration: %v", err)
	}
	fmt.Printf("   Created: %s\n", newMigration)

	// Clean up the created migration (since this is just an example)
	os.Remove(newMigration)
	fmt.Println("   (Cleaned up example migration)")
	fmt.Println()

	// =========================================================================
	// 7. Rollback Migration
	// =========================================================================
	fmt.Println("7. Rolling back last migration...")

	if err := seedup.MigrateDown(ctx, dbURL, migrationsDir); err != nil {
		log.Fatalf("Failed to rollback migration: %v", err)
	}
	fmt.Println("   Rollback complete")
	fmt.Println()

	// =========================================================================
	// 8. Flatten Migrations
	// =========================================================================
	fmt.Println("8. Flattening migrations...")
	fmt.Println("   (Consolidates all applied migrations into a single initial migration)")

	if err := seedup.Flatten(ctx, dbURL, migrationsDir); err != nil {
		log.Fatalf("Failed to flatten migrations: %v", err)
	}

	// Show what files exist after flattening
	fmt.Println("   Migration files after flatten:")
	files, _ := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	for _, f := range files {
		fmt.Printf("   - %s\n", filepath.Base(f))
	}
	fmt.Println()

	// =========================================================================
	// 9. Database Management Functions
	// =========================================================================
	fmt.Println("9. Database management functions...")
	fmt.Println("   seedup also provides database lifecycle functions:")
	fmt.Println()
	fmt.Println("   seedup.DBCreate(ctx, dbURL, opts)  - Create database")
	fmt.Println("   seedup.DBDrop(ctx, dbURL, opts)    - Drop database")
	fmt.Println("   seedup.DBSetup(ctx, opts)          - Full setup: drop + create + migrate + seed")
	fmt.Println()
	fmt.Println("   DBSetup example:")
	fmt.Print(`
   err := seedup.DBSetup(ctx, seedup.DBSetupOptions{
       DatabaseURL:   "postgres://myuser:mypass@localhost/mydb",
       MigrationsDir: "./migrations",
       SeedDir:       "./seed",
       SeedName:      "dev",
   })
`)
	fmt.Println()

	fmt.Println("=== Example Complete ===")
}

// resetSchema drops all user tables and the goose version table for a clean slate
func resetSchema(dbURL string) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	// Drop tables in correct order (respecting foreign keys)
	tables := []string{"posts", "users", "goose_db_version"}
	for _, table := range tables {
		_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
	}
	return nil
}

// getEnv returns the environment variable value or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// defaultDatabaseURL returns a database URL using the current OS user.
// This is typical for local development with Homebrew PostgreSQL.
func defaultDatabaseURL() string {
	username := "postgres" // fallback
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	return fmt.Sprintf("postgres://%s@localhost/seedup_example?sslmode=disable", username)
}
