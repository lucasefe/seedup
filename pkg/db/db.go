package db

// Manager handles database setup operations
type Manager struct{}

// New creates a new Manager
func New() *Manager {
	return &Manager{}
}

// SetupOptions configures the Setup operation.
// Setup only creates the database infrastructure (user, database, permissions).
// Use 'migrate up' and 'seed apply' separately to apply migrations and seeds.
type SetupOptions struct {
	DatabaseURL string
	AdminURL    string // Optional: URL for admin operations (defaults to postgres@host/postgres)
}
