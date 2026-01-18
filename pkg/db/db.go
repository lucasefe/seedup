package db

import (
	"github.com/lucasefe/seedup/pkg/migrate"
	"github.com/lucasefe/seedup/pkg/seed"
)

// Manager handles database setup operations
type Manager struct {
	migrator *migrate.Migrator
	seeder   *seed.Seeder
}

// New creates a new Manager
func New() *Manager {
	return &Manager{
		migrator: migrate.New(),
		seeder:   seed.New(),
	}
}

// SetupOptions configures the Setup operation
type SetupOptions struct {
	DatabaseURL   string
	AdminURL      string // Optional: URL for admin operations (defaults to postgres@host/postgres)
	MigrationsDir string
	SeedDir       string // Root seed directory (e.g., "./seed")
	SeedName      string // Name of seed set to apply (e.g., "dev" -> ./seed/dev/)
	SkipSeed      bool
}
