package seed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Apply seeds the database with data from CSV files
// It runs the initial migration, loads seed data, then runs remaining migrations
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

	// Run all remaining migrations
	fmt.Println("Running remaining migrations...")
	if err := s.migrator.Up(ctx, dbURL, migrationsDir); err != nil {
		return fmt.Errorf("running remaining migrations: %w", err)
	}

	return nil
}

func (s *Seeder) loadSeedData(ctx context.Context, dbURL, seedDir string) error {
	csvFiles, err := filepath.Glob(filepath.Join(seedDir, "*.csv"))
	if err != nil {
		return fmt.Errorf("finding CSV files: %w", err)
	}

	if len(csvFiles) == 0 {
		fmt.Println("No CSV files found in seed directory")
		return nil
	}

	// Build map of CSV file by table name
	csvByTable := make(map[string]string)
	var tables []string
	for _, csv := range csvFiles {
		table := strings.TrimSuffix(filepath.Base(csv), ".csv")
		csvByTable[table] = csv
		tables = append(tables, table)
	}

	// Get the correct import order based on foreign key dependencies
	orderedTables, err := s.getImportOrder(ctx, dbURL, tables)
	if err != nil {
		return fmt.Errorf("determining import order: %w", err)
	}

	var script bytes.Buffer

	// Begin transaction
	script.WriteString("BEGIN;\n")

	// Truncate all tables in reverse order (respects FK constraints)
	for i := len(orderedTables) - 1; i >= 0; i-- {
		table := orderedTables[i]
		script.WriteString(fmt.Sprintf("TRUNCATE TABLE %s CASCADE;\n", table))
	}

	// Generate COPY commands in dependency order
	for _, table := range orderedTables {
		csv := csvByTable[table]
		absPath, err := filepath.Abs(csv)
		if err != nil {
			return fmt.Errorf("getting absolute path for %s: %w", csv, err)
		}
		script.WriteString(fmt.Sprintf("\\COPY %s FROM '%s' WITH CSV HEADER;\n", table, absPath))
	}

	script.WriteString("COMMIT;\n")

	// Write script to temp file and execute
	tmpFile, err := os.CreateTemp("", "seed-*.sql")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script.String()); err != nil {
		return fmt.Errorf("writing seed script: %w", err)
	}
	tmpFile.Close()

	return s.exec.RunSQLFile(ctx, dbURL, tmpFile.Name())
}

// getImportOrder returns tables sorted by foreign key dependencies
// Tables with no dependencies come first, tables that depend on others come later
func (s *Seeder) getImportOrder(ctx context.Context, dbURL string, tables []string) ([]string, error) {
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

	output, err := s.exec.RunSQL(ctx, dbURL, query)
	if err != nil {
		// If we can't get dependencies, just return tables in original order
		return tables, nil
	}

	// Build dependency graph
	deps := make(map[string][]string) // table -> tables it depends on
	for _, table := range tables {
		deps[table] = nil
	}

	// Parse the output (format: "dependent | referenced" per line)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		dependent := strings.TrimSpace(parts[0])
		referenced := strings.TrimSpace(parts[1])

		// Only consider dependencies between tables we're importing
		if _, ok := deps[dependent]; ok {
			if _, ok := deps[referenced]; ok {
				deps[dependent] = append(deps[dependent], referenced)
			}
		}
	}

	// Topological sort using Kahn's algorithm
	// Count incoming edges for each table
	inDegree := make(map[string]int)
	for _, table := range tables {
		inDegree[table] = 0
	}
	for _, dependencies := range deps {
		for _, dep := range dependencies {
			inDegree[dep]++ // This is backwards - we want tables with no deps first
		}
	}

	// Actually, let's recalculate: inDegree should be count of tables this table depends on
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
