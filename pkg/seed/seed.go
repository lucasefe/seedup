package seed

import (
	"github.com/tmwinc/seedup/pkg/executor"
	"github.com/tmwinc/seedup/pkg/migrate"
)

// Seeder handles seed data operations
type Seeder struct {
	exec     executor.Executor
	migrator *migrate.Migrator
}

// New creates a new Seeder with the given executor
func New(exec executor.Executor) *Seeder {
	return &Seeder{
		exec:     exec,
		migrator: migrate.New(),
	}
}
