package pgconn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// DumpOptions configures DumpSchema.
type DumpOptions struct {
	// ExcludeTables lists tables to leave out, as "name" or "schema.name".
	ExcludeTables []string
	// ExcludeExtensions lists extensions to leave out, along with any schema
	// that exists only to hold them. Use it for extensions the source database
	// has but the target cannot install, such as aiven_extras on Aiven.
	ExcludeExtensions []string
}

// extensionMember matches rows of catalog that belong to an extension.
// CREATE EXTENSION recreates those, so dumping them separately would fail
// with "already exists"; every dump query filters them out.
func extensionMember(catalog, oid string) string {
	return fmt.Sprintf(`EXISTS (
		      SELECT 1 FROM pg_depend dep
		      WHERE dep.classid = '%s'::regclass
		        AND dep.objid = %s
		        AND dep.deptype = 'e'
		  )`, catalog, oid)
}

// DumpSchema dumps the database schema to SQL DDL statements.
// It returns SQL that can recreate the schema (excluding data).
func DumpSchema(ctx context.Context, db *sql.DB, opts DumpOptions) (string, error) {
	var parts []string

	// Build set of excluded tables for quick lookup
	excludeSet := make(map[string]bool)
	for _, t := range opts.ExcludeTables {
		excludeSet[t] = true
	}

	// A nil slice binds as SQL NULL, and "x <> ALL(NULL)" is NULL rather than
	// true, which would drop every extension. Bind an empty array instead.
	excludeExtensions := opts.ExcludeExtensions
	if excludeExtensions == nil {
		excludeExtensions = []string{}
	}

	// 1. Dump schemas (non-system)
	schemas, err := dumpSchemas(ctx, db, excludeExtensions)
	if err != nil {
		return "", fmt.Errorf("dumping schemas: %w", err)
	}
	if len(schemas) > 0 {
		parts = append(parts, "-- Schemas")
		parts = append(parts, schemas...)
		parts = append(parts, "")
	}

	// 2. Dump extensions
	extensions, err := dumpExtensions(ctx, db, excludeExtensions)
	if err != nil {
		return "", fmt.Errorf("dumping extensions: %w", err)
	}
	if len(extensions) > 0 {
		parts = append(parts, "-- Extensions")
		parts = append(parts, extensions...)
		parts = append(parts, "")
	}

	// 3. Dump enum types
	enums, err := dumpEnums(ctx, db)
	if err != nil {
		return "", fmt.Errorf("dumping enums: %w", err)
	}
	if len(enums) > 0 {
		parts = append(parts, "-- Enum types")
		parts = append(parts, enums...)
		parts = append(parts, "")
	}

	// 4. Dump domain types
	domains, err := dumpDomains(ctx, db)
	if err != nil {
		return "", fmt.Errorf("dumping domains: %w", err)
	}
	if len(domains) > 0 {
		parts = append(parts, "-- Domain types")
		parts = append(parts, domains...)
		parts = append(parts, "")
	}

	// 5. Dump composite types
	composites, err := dumpCompositeTypes(ctx, db)
	if err != nil {
		return "", fmt.Errorf("dumping composite types: %w", err)
	}
	if len(composites) > 0 {
		parts = append(parts, "-- Composite types")
		parts = append(parts, composites...)
		parts = append(parts, "")
	}

	// 6. Dump sequences
	sequences, err := dumpSequences(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping sequences: %w", err)
	}
	if len(sequences) > 0 {
		parts = append(parts, "-- Sequences")
		parts = append(parts, sequences...)
		parts = append(parts, "")
	}

	// 7. Dump PL/pgSQL functions (before tables, since table defaults may reference them)
	// These don't validate table references at creation time.
	functionsEarly, err := dumpFunctionsEarly(ctx, db)
	if err != nil {
		return "", fmt.Errorf("dumping early functions: %w", err)
	}
	if len(functionsEarly) > 0 {
		parts = append(parts, "-- Functions (PL/pgSQL)")
		parts = append(parts, functionsEarly...)
		parts = append(parts, "")
	}

	// 8. Dump tables (after PL/pgSQL functions, before SQL functions)
	tables, err := dumpTables(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping tables: %w", err)
	}
	if len(tables) > 0 {
		parts = append(parts, "-- Tables")
		parts = append(parts, tables...)
		parts = append(parts, "")
	}

	// 9. Dump SQL functions (after tables, since they validate table references at creation time)
	functionsLate, err := dumpFunctionsLate(ctx, db)
	if err != nil {
		return "", fmt.Errorf("dumping late functions: %w", err)
	}
	if len(functionsLate) > 0 {
		parts = append(parts, "-- Functions (SQL)")
		parts = append(parts, functionsLate...)
		parts = append(parts, "")
	}

	// 10. Dump primary keys
	pks, err := dumpPrimaryKeys(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping primary keys: %w", err)
	}
	if len(pks) > 0 {
		parts = append(parts, "-- Primary keys")
		parts = append(parts, pks...)
		parts = append(parts, "")
	}

	// 11. Dump unique constraints
	uniques, err := dumpUniqueConstraints(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping unique constraints: %w", err)
	}
	if len(uniques) > 0 {
		parts = append(parts, "-- Unique constraints")
		parts = append(parts, uniques...)
		parts = append(parts, "")
	}

	// 12. Dump views (after PKs and unique constraints: views that GROUP BY a
	// key column and select non-aggregated columns rely on the functional
	// dependency existing at CREATE VIEW time)
	views, err := dumpViews(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping views: %w", err)
	}
	if len(views) > 0 {
		parts = append(parts, "-- Views")
		parts = append(parts, views...)
		parts = append(parts, "")
	}

	// 13. Dump check constraints
	checks, err := dumpCheckConstraints(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping check constraints: %w", err)
	}
	if len(checks) > 0 {
		parts = append(parts, "-- Check constraints")
		parts = append(parts, checks...)
		parts = append(parts, "")
	}

	// 14. Dump foreign keys (after all tables and PKs are created)
	fks, err := dumpForeignKeys(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping foreign keys: %w", err)
	}
	if len(fks) > 0 {
		parts = append(parts, "-- Foreign keys")
		parts = append(parts, fks...)
		parts = append(parts, "")
	}

	// 15. Dump indexes (non-constraint indexes)
	indexes, err := dumpIndexes(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping indexes: %w", err)
	}
	if len(indexes) > 0 {
		parts = append(parts, "-- Indexes")
		parts = append(parts, indexes...)
		parts = append(parts, "")
	}

	// 16. Dump triggers (after functions and tables)
	triggers, err := dumpTriggers(ctx, db, excludeSet)
	if err != nil {
		return "", fmt.Errorf("dumping triggers: %w", err)
	}
	if len(triggers) > 0 {
		parts = append(parts, "-- Triggers")
		parts = append(parts, triggers...)
		parts = append(parts, "")
	}

	return strings.Join(parts, "\n"), nil
}

func dumpSchemas(ctx context.Context, db *sql.DB, excludeExtensions []string) ([]string, error) {
	// A schema that holds nothing but excluded extensions is skipped as well:
	// the extension created it, and nothing else in the dump refers to it.
	// "Nothing but" checks the catalogs whose rows carry their own extension
	// membership: relations, functions, and standalone types (rowtypes and
	// array types ride on their relation or element type instead).
	query := `
		SELECT n.nspname
		FROM pg_namespace n
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast', 'pg_temp_1', 'pg_toast_temp_1')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND n.nspname != 'public'
		  AND NOT (
		      EXISTS (
		          SELECT 1 FROM pg_extension e
		          WHERE e.extnamespace = n.oid AND e.extname = ANY($1)
		      )
		      AND NOT EXISTS (
		          SELECT 1 FROM pg_class c
		          WHERE c.relnamespace = n.oid
		            AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f')
		            AND NOT ` + extensionMember("pg_class", "c.oid") + `
		      )
		      AND NOT EXISTS (
		          SELECT 1 FROM pg_proc p
		          WHERE p.pronamespace = n.oid
		            AND NOT ` + extensionMember("pg_proc", "p.oid") + `
		      )
		      AND NOT EXISTS (
		          SELECT 1 FROM pg_type t
		          WHERE t.typnamespace = n.oid
		            AND t.typtype IN ('e', 'd', 'c')
		            AND NOT EXISTS (SELECT 1 FROM pg_class r WHERE r.oid = t.typrelid AND r.relkind <> 'c')
		            AND NOT ` + extensionMember("pg_type", "t.oid") + `
		      )
		  )
		ORDER BY n.nspname
	`

	rows, err := db.QueryContext(ctx, query, pq.Array(excludeExtensions))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		results = append(results, fmt.Sprintf("CREATE SCHEMA %s;", QuoteIdentifier(name)))
	}

	return results, rows.Err()
}

func dumpExtensions(ctx context.Context, db *sql.DB, excludeExtensions []string) ([]string, error) {
	query := `
		SELECT extname, n.nspname
		FROM pg_extension e
		JOIN pg_namespace n ON e.extnamespace = n.oid
		WHERE extname != 'plpgsql'
		  AND extname <> ALL($1)
		ORDER BY extname
	`

	rows, err := db.QueryContext(ctx, query, pq.Array(excludeExtensions))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var name, schema string
		if err := rows.Scan(&name, &schema); err != nil {
			return nil, err
		}
		results = append(results, fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s;",
			QuoteIdentifier(name), QuoteIdentifier(schema)))
	}

	return results, rows.Err()
}

func dumpEnums(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT n.nspname as schema, t.typname as name,
		       array_agg(e.enumlabel ORDER BY e.enumsortorder) as labels
		FROM pg_type t
		JOIN pg_enum e ON t.oid = e.enumtypid
		JOIN pg_namespace n ON t.typnamespace = n.oid
		WHERE t.typtype = 'e'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_type", "t.oid") + `
		GROUP BY n.nspname, t.typname
		ORDER BY n.nspname, t.typname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name string
		var labels []string

		if err := rows.Scan(&schema, &name, &labels); err != nil {
			// Try alternative scan for array
			var labelsStr string
			rows.Scan(&schema, &name, &labelsStr)
			// Parse {val1,val2,val3} format
			labelsStr = strings.Trim(labelsStr, "{}")
			if labelsStr != "" {
				labels = strings.Split(labelsStr, ",")
			}
		}

		quotedLabels := make([]string, len(labels))
		for i, l := range labels {
			quotedLabels[i] = QuoteString(l)
		}

		sql := fmt.Sprintf("CREATE TYPE %s.%s AS ENUM (\n    %s\n);",
			QuoteIdentifier(schema),
			QuoteIdentifier(name),
			strings.Join(quotedLabels, ",\n    "))
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpDomains(ctx context.Context, db *sql.DB) ([]string, error) {
	// Query domain metadata without array_agg to avoid PostgreSQL array escaping issues
	domainsQuery := `
		SELECT n.nspname as schema,
		       t.typname as name,
		       pg_catalog.format_type(t.typbasetype, t.typtypmod) as base_type,
		       t.typnotnull as not_null,
		       t.typdefault as default_value,
		       t.oid as type_oid
		FROM pg_type t
		JOIN pg_namespace n ON t.typnamespace = n.oid
		WHERE t.typtype = 'd'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_type", "t.oid") + `
		ORDER BY n.nspname, t.typname
	`

	rows, err := db.QueryContext(ctx, domainsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Query to get constraints for each domain individually
	constraintsQuery := `
		SELECT pg_get_constraintdef(c.oid, true)
		FROM pg_constraint c
		WHERE c.contypid = $1
	`

	var results []string
	for rows.Next() {
		var schema, name, baseType string
		var notNull bool
		var defaultValue sql.NullString
		var typeOID int64

		if err := rows.Scan(&schema, &name, &baseType, &notNull, &defaultValue, &typeOID); err != nil {
			return nil, err
		}

		sql := fmt.Sprintf("CREATE DOMAIN %s.%s AS %s",
			QuoteIdentifier(schema),
			QuoteIdentifier(name),
			baseType)

		if notNull {
			sql += " NOT NULL"
		}

		if defaultValue.Valid && defaultValue.String != "" {
			sql += " DEFAULT " + defaultValue.String
		}

		// Query constraints individually for this domain
		constraintRows, err := db.QueryContext(ctx, constraintsQuery, typeOID)
		if err != nil {
			return nil, fmt.Errorf("querying constraints for domain %s.%s: %w", schema, name, err)
		}

		for constraintRows.Next() {
			var constraintDef string
			if err := constraintRows.Scan(&constraintDef); err != nil {
				constraintRows.Close()
				return nil, fmt.Errorf("scanning constraint for domain %s.%s: %w", schema, name, err)
			}
			sql += "\n    " + constraintDef
		}
		constraintRows.Close()
		if err := constraintRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating constraints for domain %s.%s: %w", schema, name, err)
		}

		sql += ";"
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpCompositeTypes(ctx context.Context, db *sql.DB) ([]string, error) {
	// Get composite types, excluding auto-generated types for tables and views
	query := `
		SELECT n.nspname as schema, t.typname as name
		FROM pg_type t
		JOIN pg_namespace n ON t.typnamespace = n.oid
		WHERE t.typtype = 'c'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT EXISTS (SELECT 1 FROM pg_class c WHERE c.reltype = t.oid AND c.relkind IN ('r', 'v'))
		  AND NOT ` + extensionMember("pg_type", "t.oid") + `
		ORDER BY n.nspname, t.typname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, err
		}

		// Get attributes for this composite type
		attrQuery := `
			SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod)
			FROM pg_attribute a
			JOIN pg_type t ON a.attrelid = t.typrelid
			JOIN pg_namespace n ON t.typnamespace = n.oid
			WHERE n.nspname = $1 AND t.typname = $2
			  AND a.attnum > 0 AND NOT a.attisdropped
			ORDER BY a.attnum
		`

		attrRows, err := db.QueryContext(ctx, attrQuery, schema, name)
		if err != nil {
			return nil, err
		}

		var attrs []string
		for attrRows.Next() {
			var attrName, attrType string
			if err := attrRows.Scan(&attrName, &attrType); err != nil {
				attrRows.Close()
				return nil, err
			}
			attrs = append(attrs, fmt.Sprintf("    %s %s", QuoteIdentifier(attrName), attrType))
		}
		attrRows.Close()

		if len(attrs) > 0 {
			sql := fmt.Sprintf("CREATE TYPE %s.%s AS (\n%s\n);",
				QuoteIdentifier(schema),
				QuoteIdentifier(name),
				strings.Join(attrs, ",\n"))
			results = append(results, sql)
		}
	}

	return results, rows.Err()
}

func dumpSequences(ctx context.Context, db *sql.DB, _ map[string]bool) ([]string, error) {
	query := `
		SELECT s.schemaname, s.sequencename, s.start_value, s.increment_by, s.max_value, s.min_value, s.cache_size, s.cycle
		FROM pg_sequences s
		JOIN pg_namespace n ON n.nspname = s.schemaname
		JOIN pg_class c ON c.relnamespace = n.oid AND c.relname = s.sequencename
		WHERE s.schemaname NOT IN ('pg_catalog', 'information_schema')
		  AND s.schemaname NOT LIKE 'pg_temp_%'
		  AND s.schemaname NOT LIKE 'pg_toast_temp_%'
		  AND s.sequencename NOT LIKE 'goose_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY s.schemaname, s.sequencename
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name string
		var startVal, incBy, maxVal, minVal, cacheSize sql.NullInt64
		var cycle sql.NullBool

		if err := rows.Scan(&schema, &name, &startVal, &incBy, &maxVal, &minVal, &cacheSize, &cycle); err != nil {
			return nil, fmt.Errorf("scanning sequence: %w", err)
		}

		sql := fmt.Sprintf("CREATE SEQUENCE %s.%s",
			QuoteIdentifier(schema),
			QuoteIdentifier(name))

		if startVal.Valid {
			sql += fmt.Sprintf(" START WITH %d", startVal.Int64)
		}
		if incBy.Valid {
			sql += fmt.Sprintf(" INCREMENT BY %d", incBy.Int64)
		}
		if minVal.Valid {
			sql += fmt.Sprintf(" MINVALUE %d", minVal.Int64)
		}
		if maxVal.Valid {
			sql += fmt.Sprintf(" MAXVALUE %d", maxVal.Int64)
		}
		if cacheSize.Valid {
			sql += fmt.Sprintf(" CACHE %d", cacheSize.Int64)
		}
		if cycle.Valid && cycle.Bool {
			sql += " CYCLE"
		}
		sql += ";"
		results = append(results, sql)
	}

	return results, rows.Err()
}

// funcReferencedByColumnExpr matches functions that a column DEFAULT or
// GENERATED expression depends on (the dependency is recorded via pg_attrdef).
// Such functions must exist before the tables are created, regardless of the
// function's language.
const funcReferencedByColumnExpr = `
		EXISTS (
		    SELECT 1
		    FROM pg_depend d
		    JOIN pg_attrdef ad ON ad.oid = d.objid
		    WHERE d.refobjid = p.oid
		      AND d.refclassid = 'pg_proc'::regclass
		      AND d.classid = 'pg_attrdef'::regclass
		)`

// dumpFunctionsEarly dumps functions that must be created before tables:
// non-SQL functions (PL/pgSQL etc. don't validate table references at creation
// time and may be used by DEFAULT expressions) and any function — including SQL
// functions — that a column DEFAULT or GENERATED expression depends on.
func dumpFunctionsEarly(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT n.nspname as schema,
		       p.proname as name,
		       pg_get_functiondef(p.oid) as definition
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		JOIN pg_language l ON p.prolang = l.oid
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND p.prokind IN ('f', 'p')  -- functions and procedures
		  AND (l.lanname != 'sql' OR ` + funcReferencedByColumnExpr + `)
		  AND NOT ` + extensionMember("pg_proc", "p.oid") + `
		ORDER BY n.nspname, p.proname, p.oid
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name, definition string
		if err := rows.Scan(&schema, &name, &definition); err != nil {
			return nil, err
		}
		results = append(results, definition+";")
	}

	return results, rows.Err()
}

// dumpFunctionsLate dumps SQL language functions that are not needed before
// tables. These validate table references at creation time, so they must come
// after tables. SQL functions referenced by a column DEFAULT/GENERATED
// expression are handled by dumpFunctionsEarly instead and excluded here.
func dumpFunctionsLate(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT n.nspname as schema,
		       p.proname as name,
		       pg_get_functiondef(p.oid) as definition
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		JOIN pg_language l ON p.prolang = l.oid
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND p.prokind IN ('f', 'p')  -- functions and procedures
		  AND l.lanname = 'sql'  -- only SQL functions
		  AND NOT ` + funcReferencedByColumnExpr + `
		  AND NOT ` + extensionMember("pg_proc", "p.oid") + `
		ORDER BY n.nspname, p.proname, p.oid
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name, definition string
		if err := rows.Scan(&schema, &name, &definition); err != nil {
			return nil, err
		}
		results = append(results, definition+";")
	}

	return results, rows.Err()
}

// getGeneratedColumns returns a map of column names to their generation expressions
// for columns that are GENERATED ALWAYS AS (stored)
func getGeneratedColumns(ctx context.Context, db *sql.DB, schema, table string) (map[string]string, error) {
	query := `
		SELECT a.attname, pg_get_expr(d.adbin, d.adrelid) as generation_expr
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE n.nspname = $1
		  AND c.relname = $2
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND a.attgenerated = 's'
	`
	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var colName, expr string
		if err := rows.Scan(&colName, &expr); err != nil {
			return nil, err
		}
		result[colName] = expr
	}
	return result, rows.Err()
}

func dumpTables(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	// Get all tables
	tablesQuery := `
		SELECT n.nspname, c.relname
		FROM pg_class c
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE c.relkind IN ('r', 'p')
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND c.relname NOT LIKE 'goose_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname
	`

	tableRows, err := db.QueryContext(ctx, tablesQuery)
	if err != nil {
		return nil, err
	}
	defer tableRows.Close()

	var results []string
	for tableRows.Next() {
		var schema, tableName string
		if err := tableRows.Scan(&schema, &tableName); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		// Get generated columns info
		generatedCols, err := getGeneratedColumns(ctx, db, schema, tableName)
		if err != nil {
			return nil, fmt.Errorf("querying generated columns for %s.%s: %w", schema, tableName, err)
		}

		// Get columns for this table. format_type resolves the fully
		// schema-qualified type, including array element types in non-public
		// schemas (e.g. caregiver.checkin_call_topic[]), which the
		// information_schema udt_name column gets wrong (it returns the bare
		// internal array name like _checkin_call_topic).
		columnsQuery := `
			SELECT a.attname AS column_name,
			       format_type(a.atttypid, a.atttypmod) AS data_type,
			       CASE WHEN a.attnotnull THEN 'NO' ELSE 'YES' END AS is_nullable,
			       pg_get_expr(ad.adbin, ad.adrelid) AS column_default
			FROM pg_attribute a
			JOIN pg_class c ON c.oid = a.attrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
			WHERE n.nspname = $1 AND c.relname = $2
			  AND a.attnum > 0
			  AND NOT a.attisdropped
			ORDER BY a.attnum
		`

		colRows, err := db.QueryContext(ctx, columnsQuery, schema, tableName)
		if err != nil {
			return nil, fmt.Errorf("querying columns for %s.%s: %w", schema, tableName, err)
		}

		var columns []string
		for colRows.Next() {
			var colName, dataType string
			var isNullable, colDefault sql.NullString

			if err := colRows.Scan(&colName, &dataType, &isNullable, &colDefault); err != nil {
				colRows.Close()
				return nil, fmt.Errorf("scanning column: %w", err)
			}

			colDef := QuoteIdentifier(colName) + " " + dataType

			// Check if this is a generated column
			if genExpr, isGenerated := generatedCols[colName]; isGenerated {
				colDef += " GENERATED ALWAYS AS (" + genExpr + ") STORED"
				if isNullable.Valid && isNullable.String == "NO" {
					colDef += " NOT NULL"
				}
				// Generated columns cannot have DEFAULT, skip it
			} else {
				// Add NOT NULL if applicable
				if isNullable.Valid && isNullable.String == "NO" {
					colDef += " NOT NULL"
				}
				// Add DEFAULT if applicable
				if colDefault.Valid && colDefault.String != "" {
					colDef += " DEFAULT " + colDefault.String
				}
			}

			columns = append(columns, colDef)
		}
		colRows.Close()

		if len(columns) > 0 {
			sql := fmt.Sprintf("CREATE TABLE %s.%s (\n    %s\n);",
				QuoteIdentifier(schema),
				QuoteIdentifier(tableName),
				strings.Join(columns, ",\n    "))
			results = append(results, sql)
		}
	}

	return results, tableRows.Err()
}

func dumpViews(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname, c.relname, pg_get_viewdef(c.oid)
		FROM pg_class c
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE c.relkind = 'v'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, name, definition string
		if err := rows.Scan(&schema, &name, &definition); err != nil {
			return nil, err
		}

		fullName := schema + "." + name
		if excludeSet[fullName] || excludeSet[name] {
			continue
		}

		sql := fmt.Sprintf("CREATE VIEW %s.%s AS\n%s",
			QuoteIdentifier(schema),
			QuoteIdentifier(name),
			strings.TrimSuffix(definition, ";"))
		results = append(results, sql+";")
	}

	return results, rows.Err()
}

func dumpPrimaryKeys(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname as schema, c.relname as table_name,
		       con.conname as constraint_name,
		       pg_get_constraintdef(con.oid) as constraint_def
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE con.contype = 'p'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname, con.conname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, conName, conDef string
		if err := rows.Scan(&schema, &tableName, &conName, &conDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		sql := fmt.Sprintf("ALTER TABLE %s.%s ADD CONSTRAINT %s %s;",
			QuoteIdentifier(schema),
			QuoteIdentifier(tableName),
			QuoteIdentifier(conName),
			conDef)
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpUniqueConstraints(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname as schema, c.relname as table_name,
		       con.conname as constraint_name,
		       pg_get_constraintdef(con.oid) as constraint_def
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE con.contype = 'u'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname, con.conname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, conName, conDef string
		if err := rows.Scan(&schema, &tableName, &conName, &conDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		sql := fmt.Sprintf("ALTER TABLE %s.%s ADD CONSTRAINT %s %s;",
			QuoteIdentifier(schema),
			QuoteIdentifier(tableName),
			QuoteIdentifier(conName),
			conDef)
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpCheckConstraints(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname as schema, c.relname as table_name,
		       con.conname as constraint_name,
		       pg_get_constraintdef(con.oid) as constraint_def
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE con.contype = 'c'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname, con.conname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, conName, conDef string
		if err := rows.Scan(&schema, &tableName, &conName, &conDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		sql := fmt.Sprintf("ALTER TABLE %s.%s ADD CONSTRAINT %s %s;",
			QuoteIdentifier(schema),
			QuoteIdentifier(tableName),
			QuoteIdentifier(conName),
			conDef)
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpForeignKeys(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname as schema, c.relname as table_name,
		       con.conname as constraint_name,
		       pg_get_constraintdef(con.oid) as constraint_def
		FROM pg_constraint con
		JOIN pg_class c ON con.conrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE con.contype = 'f'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname, con.conname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, conName, conDef string
		if err := rows.Scan(&schema, &tableName, &conName, &conDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		sql := fmt.Sprintf("ALTER TABLE %s.%s ADD CONSTRAINT %s %s;",
			QuoteIdentifier(schema),
			QuoteIdentifier(tableName),
			QuoteIdentifier(conName),
			conDef)
		results = append(results, sql)
	}

	return results, rows.Err()
}

func dumpIndexes(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	// Get indexes that are not backing constraints. Within a table, keep the
	// original creation order (OID): when an INSERT violates several unique
	// indexes, Postgres reports the oldest one, so recreating them in a
	// different order changes which error callers see.
	query := `
		SELECT pg_indexes.schemaname, pg_indexes.tablename, pg_indexes.indexname, pg_indexes.indexdef
		FROM pg_indexes
		JOIN pg_namespace ON pg_namespace.nspname = pg_indexes.schemaname
		JOIN pg_class ON pg_class.relname = pg_indexes.indexname AND pg_class.relnamespace = pg_namespace.oid
		JOIN pg_index ON pg_index.indexrelid = pg_class.oid
		WHERE pg_indexes.schemaname NOT IN ('pg_catalog', 'information_schema')
		  AND pg_indexes.schemaname NOT LIKE 'pg_temp_%'
		  AND pg_indexes.schemaname NOT LIKE 'pg_toast_temp_%'
		  AND pg_indexes.indexname NOT IN (
		      SELECT conname FROM pg_constraint
		      WHERE contype IN ('p', 'u', 'x')
		  )
		  AND NOT ` + extensionMember("pg_class", "pg_index.indrelid") + `
		ORDER BY pg_indexes.schemaname, pg_indexes.tablename, pg_class.oid
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, indexName, indexDef string
		if err := rows.Scan(&schema, &tableName, &indexName, &indexDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		results = append(results, indexDef+";")
	}

	return results, rows.Err()
}

func dumpTriggers(ctx context.Context, db *sql.DB, excludeSet map[string]bool) ([]string, error) {
	query := `
		SELECT n.nspname as schema,
		       c.relname as table_name,
		       t.tgname as trigger_name,
		       pg_get_triggerdef(t.oid) as trigger_def
		FROM pg_trigger t
		JOIN pg_class c ON t.tgrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE NOT t.tgisinternal
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND n.nspname NOT LIKE 'pg_temp_%'
		  AND n.nspname NOT LIKE 'pg_toast_temp_%'
		  AND NOT ` + extensionMember("pg_class", "c.oid") + `
		ORDER BY n.nspname, c.relname, t.tgname
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var schema, tableName, triggerName, triggerDef string
		if err := rows.Scan(&schema, &tableName, &triggerName, &triggerDef); err != nil {
			return nil, err
		}

		fullName := schema + "." + tableName
		if excludeSet[fullName] || excludeSet[tableName] {
			continue
		}

		results = append(results, triggerDef+";")
	}

	return results, rows.Err()
}
