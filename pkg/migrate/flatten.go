package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasefe/seedup/pkg/pgconn"
)

// Flattener consolidates migrations into a single initial migration
type Flattener struct {
	db *sql.DB
}

// NewFlattener creates a new Flattener with the given database connection
func NewFlattener(db *sql.DB) *Flattener {
	return &Flattener{db: db}
}

// Flatten consolidates all applied migrations into a single initial migration
// It dumps the current schema and replaces all migration files with a single initial file
func (f *Flattener) Flatten(ctx context.Context, migrationsDir string) error {
	// Get all applied migration versions
	versions, err := f.getAppliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("getting applied versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No applied migrations found, skipping flatten")
		return nil
	}

	// Get the latest version for the new initial migration
	latestVersion := versions[len(versions)-1]

	// Dump the current schema using our custom schema dumper
	schema, err := f.dumpSchema(ctx)
	if err != nil {
		return fmt.Errorf("dumping schema: %w", err)
	}

	// Delete all existing migration files
	for _, version := range versions {
		pattern := filepath.Join(migrationsDir, version+"_*.sql")
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return fmt.Errorf("removing migration file %s: %w", match, err)
			}
		}
	}

	// Create the new initial migration
	initialPath := filepath.Join(migrationsDir, latestVersion+"_initial.sql")
	if err := f.writeInitialMigration(initialPath, schema); err != nil {
		return fmt.Errorf("writing initial migration: %w", err)
	}

	return nil
}

func (f *Flattener) getAppliedVersions(ctx context.Context) ([]string, error) {
	// First check if the goose_db_version table exists
	checkQuery := `SELECT EXISTS (
		SELECT FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name = 'goose_db_version'
	)`
	var exists bool
	if err := f.db.QueryRowContext(ctx, checkQuery).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		// Table doesn't exist, no migrations have been run
		return nil, nil
	}

	rows, err := f.db.QueryContext(ctx,
		"SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY version_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		version = strings.TrimSpace(version)
		if version != "" {
			versions = append(versions, version)
		}
	}

	return versions, rows.Err()
}

func (f *Flattener) dumpSchema(ctx context.Context) (string, error) {
	// Use our custom schema dumper, excluding goose tables
	excludeTables := []string{
		"goose_db_version",
		"public.goose_db_version",
	}

	schema, err := pgconn.DumpSchema(ctx, f.db, excludeTables)
	if err != nil {
		return "", err
	}

	return schema, nil
}

func (f *Flattener) writeInitialMigration(path, schema string) error {
	var buf bytes.Buffer

	buf.WriteString("-- +goose Up\n")
	buf.WriteString("-- +goose StatementBegin\n")
	buf.WriteString(schema)
	buf.WriteString("\n-- +goose StatementEnd\n")

	return os.WriteFile(path, buf.Bytes(), 0644)
}
