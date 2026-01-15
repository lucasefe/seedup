package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
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
	verbose bool
	stdout  io.Writer
}

// Option configures a Migrator
type Option func(*Migrator)

// WithVerbose enables verbose output
func WithVerbose(v bool) Option {
	return func(m *Migrator) {
		m.verbose = v
	}
}

// WithStdout sets the stdout writer
func WithStdout(w io.Writer) Option {
	return func(m *Migrator) {
		m.stdout = w
	}
}

// New creates a new Migrator with the given options
func New(opts ...Option) *Migrator {
	m := &Migrator{
		stdout: os.Stdout,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context, dbURL, migrationsDir string) error {
	db, err := m.openDB(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	m.configureGoose()
	return goose.UpContext(ctx, db, migrationsDir)
}

// UpByOne runs a single pending migration
func (m *Migrator) UpByOne(ctx context.Context, dbURL, migrationsDir string) error {
	db, err := m.openDB(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	m.configureGoose()
	return goose.UpByOneContext(ctx, db, migrationsDir)
}

// UpByOneAllowNoop runs a single pending migration, but doesn't fail if no migrations are pending
func (m *Migrator) UpByOneAllowNoop(ctx context.Context, dbURL, migrationsDir string) error {
	db, err := m.openDB(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	m.configureGoose()
	err = goose.UpByOneContext(ctx, db, migrationsDir)
	if err != nil {
		if errors.Is(err, goose.ErrNoNextVersion) {
			return nil
		}
		return err
	}
	return nil
}

// Down rolls back the last migration
func (m *Migrator) Down(ctx context.Context, dbURL, migrationsDir string) error {
	db, err := m.openDB(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	m.configureGoose()
	return goose.DownContext(ctx, db, migrationsDir)
}

// Status shows the status of all migrations
func (m *Migrator) Status(ctx context.Context, dbURL, migrationsDir string) error {
	db, err := m.openDB(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	m.configureGoose()
	return goose.StatusContext(ctx, db, migrationsDir)
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

func (m *Migrator) openDB(dbURL string) (*sql.DB, error) {
	dbURL = ensureSSLMode(dbURL)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return db, nil
}

func (m *Migrator) configureGoose() {
	goose.SetDialect("postgres")
	goose.SetVerbose(m.verbose)
}

// ensureSSLMode adds sslmode=disable if no sslmode is specified in the URL.
func ensureSSLMode(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		return dbURL
	}

	q := u.Query()
	if q.Get("sslmode") == "" {
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
		return u.String()
	}

	return dbURL
}
