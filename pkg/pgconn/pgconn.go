package pgconn

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "github.com/lib/pq"
)

// Open opens a database connection with SSL mode handling.
func Open(dbURL string) (*sql.DB, error) {
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

// OpenTx opens a database connection and starts a transaction.
func OpenTx(ctx context.Context, dbURL string) (*sql.DB, *sql.Tx, error) {
	db, err := Open(dbURL)
	if err != nil {
		return nil, nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("starting transaction: %w", err)
	}

	return db, tx, nil
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

// QuoteIdentifier quotes a PostgreSQL identifier (table name, column name, etc.)
// to prevent SQL injection and handle reserved words.
func QuoteIdentifier(s string) string {
	return `"` + escapeDoubleQuotes(s) + `"`
}

// escapeDoubleQuotes escapes double quotes in identifiers.
func escapeDoubleQuotes(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			result = append(result, '"', '"')
		} else {
			result = append(result, s[i])
		}
	}
	return string(result)
}
