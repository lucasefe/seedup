package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasefe/seedup/pkg/pgconn"
)

// Drop drops the database specified in the DATABASE_URL
func (m *Manager) Drop(ctx context.Context, dbURL, adminURL string) error {
	cfg, err := ParseDatabaseURL(dbURL)
	if err != nil {
		return err
	}

	if adminURL == "" {
		adminURL = cfg.AdminURL()
	}

	db, err := pgconn.Open(adminURL)
	if err != nil {
		return fmt.Errorf("connecting to admin database: %w", err)
	}
	defer db.Close()

	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(cfg.Database))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("dropping database: %w", err)
	}

	return nil
}

// Create creates the database specified in the DATABASE_URL
func (m *Manager) Create(ctx context.Context, dbURL, adminURL string) error {
	cfg, err := ParseDatabaseURL(dbURL)
	if err != nil {
		return err
	}

	if adminURL == "" {
		adminURL = cfg.AdminURL()
	}

	db, err := pgconn.Open(adminURL)
	if err != nil {
		return fmt.Errorf("connecting to admin database: %w", err)
	}
	defer db.Close()

	// Check if database already exists
	checkQuery := "SELECT 1 FROM pg_database WHERE datname = $1"
	var exists int
	err = db.QueryRowContext(ctx, checkQuery, cfg.Database).Scan(&exists)
	if err == nil && exists == 1 {
		return nil // Database already exists
	}

	query := fmt.Sprintf("CREATE DATABASE %s", quoteIdent(cfg.Database))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	return nil
}

// CreateUser creates the database user if it doesn't exist
func (m *Manager) CreateUser(ctx context.Context, dbURL, adminURL string) error {
	cfg, err := ParseDatabaseURL(dbURL)
	if err != nil {
		return err
	}

	if adminURL == "" {
		adminURL = cfg.AdminURL()
	}

	db, err := pgconn.Open(adminURL)
	if err != nil {
		return fmt.Errorf("connecting to admin database: %w", err)
	}
	defer db.Close()

	// Check if user already exists
	checkQuery := "SELECT 1 FROM pg_roles WHERE rolname = $1"
	var exists int
	err = db.QueryRowContext(ctx, checkQuery, cfg.User).Scan(&exists)
	if err == nil && exists == 1 {
		return nil // User already exists
	}

	// Note: We can't use parameterized queries for CREATE USER, but we validate the username
	query := fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", quoteIdent(cfg.User), escapePassword(cfg.Password))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	return nil
}

// SetupPermissions grants necessary permissions on the database
func (m *Manager) SetupPermissions(ctx context.Context, dbURL, adminURL string) error {
	cfg, err := ParseDatabaseURL(dbURL)
	if err != nil {
		return err
	}

	if adminURL == "" {
		adminURL = cfg.AdminURL()
	}

	db, err := pgconn.Open(adminURL)
	if err != nil {
		return fmt.Errorf("connecting to admin database: %w", err)
	}
	defer db.Close()

	// Grant all on the database
	grantDBQuery := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s",
		quoteIdent(cfg.Database), quoteIdent(cfg.User))
	if _, err := db.ExecContext(ctx, grantDBQuery); err != nil {
		return fmt.Errorf("granting database privileges: %w", err)
	}

	// Make user owner of the database
	ownerQuery := fmt.Sprintf("ALTER DATABASE %s OWNER TO %s",
		quoteIdent(cfg.Database), quoteIdent(cfg.User))
	if _, err := db.ExecContext(ctx, ownerQuery); err != nil {
		return fmt.Errorf("setting database owner: %w", err)
	}

	return nil
}

// Setup performs a full database setup: create user, drop, create db, set permissions.
// This does NOT run migrations or apply seeds - use 'migrate up' and 'seed apply' separately.
func (m *Manager) Setup(ctx context.Context, opts SetupOptions) error {
	cfg, err := ParseDatabaseURL(opts.DatabaseURL)
	if err != nil {
		return err
	}

	adminURL := opts.AdminURL
	if adminURL == "" {
		adminURL = cfg.AdminURL()
	}

	// 1. Create user if not exists (do this first so DROP doesn't fail if user doesn't exist)
	fmt.Printf("Creating user '%s' if not exists...\n", cfg.User)
	if err := m.CreateUser(ctx, opts.DatabaseURL, adminURL); err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	// 2. Drop database if exists
	fmt.Printf("Dropping database '%s' if exists...\n", cfg.Database)
	if err := m.Drop(ctx, opts.DatabaseURL, adminURL); err != nil {
		return fmt.Errorf("dropping database: %w", err)
	}

	// 3. Create database
	fmt.Printf("Creating database '%s'...\n", cfg.Database)
	if err := m.Create(ctx, opts.DatabaseURL, adminURL); err != nil {
		return fmt.Errorf("creating database: %w", err)
	}

	// 4. Setup permissions
	fmt.Println("Setting up permissions...")
	if err := m.SetupPermissions(ctx, opts.DatabaseURL, adminURL); err != nil {
		return fmt.Errorf("setting up permissions: %w", err)
	}

	return nil
}

// quoteIdent quotes a PostgreSQL identifier to prevent SQL injection
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// escapePassword escapes single quotes in passwords for SQL
func escapePassword(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
