package seed

import (
	"github.com/tmwinc/seedup/pkg/migrate"
)

// Seeder handles seed data operations
type Seeder struct {
	migrator *migrate.Migrator
}

// New creates a new Seeder
func New() *Seeder {
	return &Seeder{
		migrator: migrate.New(),
	}
}
