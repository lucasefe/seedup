package seed

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmwinc/seedup/pkg/migrate"
	"github.com/tmwinc/seedup/pkg/pgconn"
)

// CreateOptions configures the seed creation process
type CreateOptions struct {
	DryRun bool
}

// Create creates seed data from a database
// It dumps the schema, flattens migrations, and exports seed data to SQL files
func (s *Seeder) Create(ctx context.Context, dbURL, migrationsDir, seedDir, queryFile string, opts CreateOptions) error {
	// Ensure seed directory exists
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		return fmt.Errorf("creating seed directory: %w", err)
	}

	// Open database connection
	db, err := pgconn.Open(dbURL)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Get all tables in the database
	tables, err := s.getTables(ctx, db)
	if err != nil {
		return fmt.Errorf("getting tables: %w", err)
	}

	// Build and execute the seed data extraction script
	tempDir, err := os.MkdirTemp("", "seedup-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := s.extractSeedData(ctx, db, tables, queryFile, tempDir); err != nil {
		return fmt.Errorf("extracting seed data: %w", err)
	}

	if opts.DryRun {
		fmt.Println("Dry run mode - not modifying any files")
		return nil
	}

	// Flatten migrations
	flattener := migrate.NewFlattener(db)
	if err := flattener.Flatten(ctx, migrationsDir); err != nil {
		return fmt.Errorf("flattening migrations: %w", err)
	}

	// Clean old seed files (both CSV and SQL for migration)
	oldCSVs, _ := filepath.Glob(filepath.Join(seedDir, "*.csv"))
	for _, csv := range oldCSVs {
		os.Remove(csv)
	}
	oldSQLs, _ := filepath.Glob(filepath.Join(seedDir, "*.sql"))
	for _, sql := range oldSQLs {
		os.Remove(sql)
	}

	// Move new SQL files
	newSQLs, _ := filepath.Glob(filepath.Join(tempDir, "*.sql"))
	for _, sqlFile := range newSQLs {
		dest := filepath.Join(seedDir, filepath.Base(sqlFile))
		data, err := os.ReadFile(sqlFile)
		if err != nil {
			return fmt.Errorf("reading %s: %w", sqlFile, err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
	}

	return nil
}

type tableInfo struct {
	Schema string
	Name   string
}

func (s *Seeder) getTables(ctx context.Context, db *sql.DB) ([]tableInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT schemaname, tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname NOT IN ('information_schema', 'pg_catalog')
		AND schemaname NOT LIKE 'pg_temp%'
		AND tablename <> 'goose_db_version'
		ORDER BY schemaname, tablename
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tables: %w", err)
	}
	defer rows.Close()

	var tables []tableInfo
	for rows.Next() {
		var t tableInfo
		if err := rows.Scan(&t.Schema, &t.Name); err != nil {
			return nil, fmt.Errorf("scanning table info: %w", err)
		}
		tables = append(tables, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tables: %w", err)
	}

	return tables, nil
}

func (s *Seeder) extractSeedData(ctx context.Context, db *sql.DB, tables []tableInfo, queryFile, outputDir string) error {
	// Start a transaction for temp table visibility
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Create temp tables for each real table
	for _, t := range tables {
		tempTable := fmt.Sprintf(`"seed.%s.%s"`, t.Schema, t.Name)
		createSQL := fmt.Sprintf(
			`CREATE TEMP TABLE %s (LIKE %s.%s INCLUDING ALL)`,
			tempTable,
			pgconn.QuoteIdentifier(t.Schema),
			pgconn.QuoteIdentifier(t.Name),
		)
		if _, err := tx.ExecContext(ctx, createSQL); err != nil {
			return fmt.Errorf("creating temp table for %s.%s: %w", t.Schema, t.Name, err)
		}
	}

	// Execute the user's seed query file which populates the temp tables
	if queryFile != "" {
		queryContent, err := os.ReadFile(queryFile)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Warning: seed query file '%s' not found, proceeding without custom queries\n", queryFile)
			} else {
				return fmt.Errorf("reading query file: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, string(queryContent)); err != nil {
				return fmt.Errorf("executing seed query file: %w", err)
			}
		}
	}

	// Export each temp table to SQL INSERT file
	for _, t := range tables {
		if err := s.exportTableToSQL(ctx, tx, t, outputDir); err != nil {
			return fmt.Errorf("exporting table %s.%s: %w", t.Schema, t.Name, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

func (s *Seeder) exportTableToSQL(ctx context.Context, tx *sql.Tx, t tableInfo, outputDir string) error {
	tempTableName := fmt.Sprintf(`pg_temp.seed.%s.%s`, t.Schema, t.Name)
	tempTableQuoted := fmt.Sprintf(`"seed.%s.%s"`, t.Schema, t.Name)

	// Get column info for the temp table
	columns, err := pgconn.GetColumnInfo(ctx, tx, tempTableName)
	if err != nil {
		return fmt.Errorf("getting column info: %w", err)
	}

	if len(columns) == 0 {
		// No columns found, skip this table
		fmt.Printf("Warning: no columns found for table %s.%s\n", t.Schema, t.Name)
		return nil
	}

	// Query all rows from the temp table
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", tempTableQuoted))
	if err != nil {
		return fmt.Errorf("querying temp table: %w", err)
	}
	defer rows.Close()

	// Build column names list for INSERT statements
	colNames := make([]string, len(columns))
	for i, col := range columns {
		colNames[i] = pgconn.QuoteIdentifier(col.Name)
	}
	colNamesStr := strings.Join(colNames, ", ")

	// Build INSERT statements
	var inserts []string
	for rows.Next() {
		// Create scan destinations
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("scanning row: %w", err)
		}

		// Serialize values
		serialized := pgconn.SerializeRow(values, columns)
		valuesStr := strings.Join(serialized, ", ")

		insert := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s);",
			pgconn.QuoteIdentifier(t.Schema),
			pgconn.QuoteIdentifier(t.Name),
			colNamesStr,
			valuesStr,
		)
		inserts = append(inserts, insert)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating rows: %w", err)
	}

	// Write to SQL file (even if empty, to indicate the table exists)
	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.%s.sql", t.Schema, t.Name))

	var content string
	if len(inserts) > 0 {
		content = strings.Join(inserts, "\n") + "\n"
	} else {
		content = fmt.Sprintf("-- No data for table %s.%s\n", t.Schema, t.Name)
	}

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing SQL file: %w", err)
	}

	fmt.Printf("Exported %s.%s (%d rows)\n", t.Schema, t.Name, len(inserts))
	return nil
}
