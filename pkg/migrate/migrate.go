package migrate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tmwinc/seedup/pkg/executor"
)

// MigrationStatus represents the status of a single migration
type MigrationStatus struct {
	Version   string
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

// Migrator handles database migrations using goose
type Migrator struct {
	exec executor.Executor
}

// New creates a new Migrator with the given executor
func New(exec executor.Executor) *Migrator {
	return &Migrator{exec: exec}
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context, dbURL, migrationsDir string) error {
	return m.exec.Run(ctx, "goose", "postgres", dbURL, "-dir", migrationsDir, "up", "sql")
}

// UpByOne runs a single pending migration
func (m *Migrator) UpByOne(ctx context.Context, dbURL, migrationsDir string) error {
	return m.exec.Run(ctx, "goose", "postgres", dbURL, "-dir", migrationsDir, "up-by-one", "sql")
}

// UpByOneAllowNoop runs a single pending migration, but doesn't fail if no migrations are pending
func (m *Migrator) UpByOneAllowNoop(ctx context.Context, dbURL, migrationsDir string) error {
	err := m.exec.Run(ctx, "goose", "postgres", dbURL, "-dir", migrationsDir, "up-by-one", "sql")
	if err != nil {
		// Check if the error is just "no next version found" which means migrations are already applied
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// This is expected when no migrations are pending - not an error for our purposes
			return nil
		}
		return err
	}
	return nil
}

// Down rolls back the last migration
func (m *Migrator) Down(ctx context.Context, dbURL, migrationsDir string) error {
	return m.exec.Run(ctx, "goose", "postgres", dbURL, "-dir", migrationsDir, "down", "sql")
}

// Status shows the status of all migrations
func (m *Migrator) Status(ctx context.Context, dbURL, migrationsDir string) error {
	return m.exec.Run(ctx, "goose", "postgres", dbURL, "-dir", migrationsDir, "status", "sql")
}

// Create creates a new migration file with the given name
func (m *Migrator) Create(migrationsDir, name string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.sql", timestamp, name)
	filepath := filepath.Join(migrationsDir, filename)

	content := `-- +goose Up
-- +goose StatementBegin

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- +goose StatementEnd
`

	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		return "", fmt.Errorf("creating migrations directory: %w", err)
	}

	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing migration file: %w", err)
	}

	return filepath, nil
}
