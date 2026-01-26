package seed

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucasefe/seedup/pkg/pgconn"
)

// Apply seeds the database with data from SQL files.
// It runs the initial migration (to establish the schema the seed was created against),
// then loads seed data. Run 'migrate up' separately to apply remaining migrations.
func (s *Seeder) Apply(ctx context.Context, dbURL, migrationsDir, seedDir string) error {
	// Run the initial migration (schema at point of creating seed)
	// Use UpByOneAllowNoop to handle the case where migrations are already applied
	fmt.Println("Running initial migration (if pending)...")
	if err := s.migrator.UpByOneAllowNoop(ctx, dbURL, migrationsDir); err != nil {
		return fmt.Errorf("running initial migration: %w", err)
	}

	// Build and execute the seed script
	fmt.Println("Seeding database...")
	if err := s.loadSeedData(ctx, dbURL, seedDir); err != nil {
		return fmt.Errorf("loading seed data: %w", err)
	}

	return nil
}

func (s *Seeder) loadSeedData(ctx context.Context, dbURL, seedDir string) error {
	// Open database connection
	db, err := pgconn.Open(dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Look for load.sql (new format)
	seedFile := filepath.Join(seedDir, "load.sql")
	content, err := os.ReadFile(seedFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Check for legacy per-table files
			legacyFiles, _ := filepath.Glob(filepath.Join(seedDir, "*.*.sql"))
			if len(legacyFiles) > 0 {
				return fmt.Errorf("found legacy per-table SQL files but no load.sql. Please run 'seed create' to regenerate seed files in the new format")
			}
			fmt.Println("No load.sql found in seed directory")
			return nil
		}
		return fmt.Errorf("reading seed file %s: %w", seedFile, err)
	}

	// Extract table names from comments (e.g., "-- Table: public.users")
	var tables []string
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "-- Table: ") {
			tableName := strings.TrimPrefix(line, "-- Table: ")
			tables = append(tables, tableName)
		}
	}

	if len(tables) == 0 {
		fmt.Println("No tables found in seed file")
		return nil
	}

	// Get the correct import order based on foreign key dependencies
	orderedTables, err := s.getImportOrder(ctx, db, tables)
	if err != nil {
		return fmt.Errorf("determining import order: %w", err)
	}

	// Start transaction for atomic seed loading
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Truncate all tables in reverse order (respects FK constraints)
	for i := len(orderedTables) - 1; i >= 0; i-- {
		table := orderedTables[i]
		truncateSQL := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		if _, err := tx.ExecContext(ctx, truncateSQL); err != nil {
			return fmt.Errorf("truncating table %s: %w", table, err)
		}
	}

	// Execute the entire seed file
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("executing seed file: %w", err)
	}
	fmt.Printf("Loaded seed data for %d tables\n", len(tables))

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// getImportOrder returns tables sorted by foreign key dependencies
// Tables with no dependencies come first, tables that depend on others come later
func (s *Seeder) getImportOrder(ctx context.Context, db *sql.DB, tables []string) ([]string, error) {
	if len(tables) == 0 {
		return nil, nil
	}

	// Query to get FK dependencies for the given tables
	// Returns pairs of (dependent_table, referenced_table)
	query := `
		SELECT DISTINCT
			tc.table_schema || '.' || tc.table_name as dependent,
			ccu.table_schema || '.' || ccu.table_name as referenced
		FROM information_schema.table_constraints tc
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// If we can't get dependencies, just return tables in original order
		return tables, nil
	}
	defer rows.Close()

	// Build dependency graph
	deps := make(map[string][]string) // table -> tables it depends on
	for _, table := range tables {
		deps[table] = nil
	}

	for rows.Next() {
		var dependent, referenced string
		if err := rows.Scan(&dependent, &referenced); err != nil {
			continue
		}

		// Only consider dependencies between tables we're importing
		if _, ok := deps[dependent]; ok {
			if _, ok := deps[referenced]; ok {
				deps[dependent] = append(deps[dependent], referenced)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return tables, nil
	}

	// Topological sort using Kahn's algorithm
	// Count incoming edges for each table
	inDegree := make(map[string]int)
	for _, table := range tables {
		inDegree[table] = 0
	}

	// Actually, let's calculate: inDegree should be count of tables this table depends on
	for table := range inDegree {
		inDegree[table] = len(deps[table])
	}

	// Start with tables that have no dependencies
	var queue []string
	for _, table := range tables {
		if inDegree[table] == 0 {
			queue = append(queue, table)
		}
	}
	sort.Strings(queue) // Stable ordering

	var result []string
	for len(queue) > 0 {
		// Take first from queue
		table := queue[0]
		queue = queue[1:]
		result = append(result, table)

		// For each table that depends on this one, decrease its in-degree
		for other, dependencies := range deps {
			for _, dep := range dependencies {
				if dep == table {
					inDegree[other]--
					if inDegree[other] == 0 {
						queue = append(queue, other)
						sort.Strings(queue) // Keep stable ordering
					}
				}
			}
		}
	}

	// If we couldn't order all tables (cycle?), append remaining
	if len(result) < len(tables) {
		tableSet := make(map[string]bool)
		for _, t := range result {
			tableSet[t] = true
		}
		for _, t := range tables {
			if !tableSet[t] {
				result = append(result, t)
			}
		}
	}

	return result, nil
}
