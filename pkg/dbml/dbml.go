package dbml

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/lucasefe/dbml"
)

// Generator handles DBML generation from PostgreSQL databases
type Generator struct{}

// Options configures DBML generation
type Options struct {
	Output        string   // Output file (empty = stdout)
	Schemas       []string // Schemas to include (empty = default)
	ExcludeTables []string // Tables to exclude
	AllSchemas    bool     // Include all non-system schemas
}

// New creates a new Generator
func New() *Generator {
	return &Generator{}
}

// Generate creates a DBML file from the database schema
func (g *Generator) Generate(ctx context.Context, dbURL string, opts Options) error {
	result, err := g.GenerateString(ctx, dbURL, opts)
	if err != nil {
		return err
	}

	if opts.Output == "" {
		fmt.Print(result)
		return nil
	}

	return os.WriteFile(opts.Output, []byte(result), 0644)
}

// GenerateString generates DBML and returns it as a string
func (g *Generator) GenerateString(ctx context.Context, dbURL string, opts Options) (string, error) {
	dbURL = ensureSSLMode(dbURL)

	config := &dbml.Config{
		Schemas:           opts.Schemas,
		ExcludeTables:     opts.ExcludeTables,
		IncludeAllSchemas: opts.AllSchemas,
	}

	return dbml.GenerateFromConnectionString(dbURL, config)
}

// ensureSSLMode adds sslmode=disable if no sslmode is specified in the URL.
// This is needed because lib/pq (used by dbml) requires explicit SSL config,
// unlike psql which defaults to "prefer" and gracefully falls back.
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
