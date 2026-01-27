package db

import (
	"fmt"
	"net/url"
	"os/user"
	"strings"
)

// DBConfig holds parsed database connection details
type DBConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Database string
	SSLMode  string
}

// ParseDatabaseURL parses a PostgreSQL connection URL
func ParseDatabaseURL(rawURL string) (*DBConfig, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL: %w", err)
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("invalid scheme: expected postgres or postgresql, got %s", u.Scheme)
	}

	password, _ := u.User.Password()

	port := u.Port()
	if port == "" {
		port = "5432"
	}

	database := strings.TrimPrefix(u.Path, "/")

	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}

	return &DBConfig{
		User:     u.User.Username(),
		Password: password,
		Host:     u.Hostname(),
		Port:     port,
		Database: database,
		SSLMode:  sslMode,
	}, nil
}

// AdminURL returns a URL pointing to the 'postgres' database using the current system user.
// This is used for admin operations like CREATE DATABASE, CREATE USER, etc.
// On most local setups (macOS/Homebrew), the superuser is the system username.
func (c *DBConfig) AdminURL() string {
	adminUser := "postgres"
	if u, err := user.Current(); err == nil && u.Username != "" {
		adminUser = u.Username
	}
	return fmt.Sprintf("postgres://%s@%s:%s/postgres?sslmode=%s",
		adminUser, c.Host, c.Port, c.SSLMode)
}

// URLWithDatabase returns a URL with a different database name
func (c *DBConfig) URLWithDatabase(dbName string) string {
	if c.Password != "" {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			c.User, url.QueryEscape(c.Password), c.Host, c.Port, dbName, c.SSLMode)
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		c.User, c.Host, c.Port, dbName, c.SSLMode)
}

// AdminURLForDatabase returns an admin URL pointing to a specific database.
// This is used for admin operations that need to run on the target database.
func (c *DBConfig) AdminURLForDatabase(dbName string) string {
	adminUser := "postgres"
	if u, err := user.Current(); err == nil && u.Username != "" {
		adminUser = u.Username
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
		adminUser, c.Host, c.Port, dbName, c.SSLMode)
}
