package pgconn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ColumnInfo holds metadata about a database column.
type ColumnInfo struct {
	Name     string
	DataType string // PostgreSQL data type (e.g., "integer", "text", "numrange")
}

// GetColumnInfo retrieves column information for a table.
func GetColumnInfo(ctx context.Context, db interface{ QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) }, tableName string) ([]ColumnInfo, error) {
	// Handle temp table names like pg_temp.seed.public.users
	isTempTable := false
	schema := "public"
	table := tableName

	// Check if tableName contains schema prefix
	if strings.HasPrefix(tableName, "pg_temp.") {
		// Temp table: pg_temp.seed.public.users -> table = seed.public.users
		isTempTable = true
		table = strings.TrimPrefix(tableName, "pg_temp.")
	} else if strings.Contains(tableName, ".") {
		parts := strings.SplitN(tableName, ".", 2)
		schema = parts[0]
		table = parts[1]
	}

	// Remove quotes from table name for the query
	table = strings.Trim(table, `"`)

	var query string
	var args []interface{}

	if isTempTable {
		// For temp tables, use LIKE pattern to match pg_temp_N schemas
		query = `
			SELECT a.attname AS column_name,
			       format_type(a.atttypid, a.atttypmod) AS data_type
			FROM pg_attribute a
			JOIN pg_class c ON a.attrelid = c.oid
			JOIN pg_namespace n ON c.relnamespace = n.oid
			WHERE c.relname = $1
			  AND n.nspname LIKE 'pg_temp_%'
			  AND a.attnum > 0
			  AND NOT a.attisdropped
			ORDER BY a.attnum
		`
		args = []interface{}{table}
	} else {
		query = `
			SELECT a.attname AS column_name,
			       format_type(a.atttypid, a.atttypmod) AS data_type
			FROM pg_attribute a
			JOIN pg_class c ON a.attrelid = c.oid
			JOIN pg_namespace n ON c.relnamespace = n.oid
			WHERE c.relname = $1
			  AND n.nspname = $2
			  AND a.attnum > 0
			  AND NOT a.attisdropped
			ORDER BY a.attnum
		`
		args = []interface{}{table, schema}
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying column info for %s: %w", tableName, err)
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		if err := rows.Scan(&col.Name, &col.DataType); err != nil {
			return nil, fmt.Errorf("scanning column info: %w", err)
		}
		cols = append(cols, col)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating column info: %w", err)
	}

	return cols, nil
}

// SerializeValue converts a Go value to a PostgreSQL literal string for use in INSERT statements.
func SerializeValue(value interface{}, pgType string) string {
	if value == nil {
		return "NULL"
	}

	// Handle sql.RawBytes and []byte which are common scan targets
	switch v := value.(type) {
	case []byte:
		if v == nil {
			return "NULL"
		}
		// Check if this is a bytea type
		if strings.HasPrefix(pgType, "bytea") {
			return fmt.Sprintf("'\\x%x'", v)
		}
		// Use dollar-quoting for arrays to avoid escaping issues with composite types
		// that contain embedded quotes (e.g., tax_type[] with text fields)
		if strings.HasSuffix(strings.ToLower(pgType), "[]") {
			return quoteDollar(string(v))
		}
		// Otherwise treat as string
		return QuoteString(string(v))
	case *interface{}:
		if v == nil {
			return "NULL"
		}
		return SerializeValue(*v, pgType)
	}

	// Normalize the type for comparison
	normalizedType := strings.ToLower(pgType)

	switch {
	// NULL handling
	case value == nil:
		return "NULL"

	// Boolean
	case normalizedType == "boolean" || normalizedType == "bool":
		return fmt.Sprintf("%v", value)

	// Integer types
	case normalizedType == "integer" || normalizedType == "int" ||
		normalizedType == "bigint" || normalizedType == "smallint" ||
		normalizedType == "serial" || normalizedType == "bigserial":
		return fmt.Sprintf("%v", value)

	// Floating point types
	case normalizedType == "real" || normalizedType == "double precision" ||
		normalizedType == "numeric" || normalizedType == "decimal" ||
		strings.HasPrefix(normalizedType, "numeric("):
		return fmt.Sprintf("%v", value)

	// Date/time types
	case normalizedType == "timestamp without time zone" ||
		normalizedType == "timestamp" ||
		strings.HasPrefix(normalizedType, "timestamp("):
		if t, ok := value.(time.Time); ok {
			return QuoteString(t.Format("2006-01-02 15:04:05.999999"))
		}
		return QuoteString(fmt.Sprintf("%v", value))

	case normalizedType == "timestamp with time zone" ||
		normalizedType == "timestamptz" ||
		strings.HasPrefix(normalizedType, "timestamp(") && strings.Contains(normalizedType, "with time zone"):
		if t, ok := value.(time.Time); ok {
			return QuoteString(t.Format("2006-01-02 15:04:05.999999-07:00"))
		}
		return QuoteString(fmt.Sprintf("%v", value))

	case normalizedType == "date":
		if t, ok := value.(time.Time); ok {
			return QuoteString(t.Format("2006-01-02"))
		}
		return QuoteString(fmt.Sprintf("%v", value))

	case normalizedType == "time without time zone" ||
		normalizedType == "time" ||
		strings.HasPrefix(normalizedType, "time("):
		if t, ok := value.(time.Time); ok {
			return QuoteString(t.Format("15:04:05.999999"))
		}
		return QuoteString(fmt.Sprintf("%v", value))

	case normalizedType == "time with time zone" || normalizedType == "timetz":
		if t, ok := value.(time.Time); ok {
			return QuoteString(t.Format("15:04:05.999999-07:00"))
		}
		return QuoteString(fmt.Sprintf("%v", value))

	case normalizedType == "interval":
		return QuoteString(fmt.Sprintf("%v", value))

	// UUID
	case normalizedType == "uuid":
		return QuoteString(fmt.Sprintf("%v", value))

	// JSON types
	case normalizedType == "json" || normalizedType == "jsonb":
		return QuoteString(fmt.Sprintf("%v", value))

	// Range types
	case normalizedType == "numrange" ||
		normalizedType == "int4range" ||
		normalizedType == "int8range" ||
		normalizedType == "tsrange" ||
		normalizedType == "tstzrange" ||
		normalizedType == "daterange":
		return QuoteString(fmt.Sprintf("%v", value))

	// Array types (type ends with [])
	// Use dollar-quoting for arrays to avoid escaping issues with composite types
	// that contain embedded quotes (e.g., tax_type[] with text fields)
	case strings.HasSuffix(normalizedType, "[]"):
		return quoteDollar(fmt.Sprintf("%v", value))

	// Text/string types
	case normalizedType == "text" ||
		normalizedType == "character varying" ||
		strings.HasPrefix(normalizedType, "character varying(") ||
		normalizedType == "varchar" ||
		strings.HasPrefix(normalizedType, "varchar(") ||
		normalizedType == "character" ||
		strings.HasPrefix(normalizedType, "character(") ||
		normalizedType == "char" ||
		strings.HasPrefix(normalizedType, "char(") ||
		normalizedType == "name" ||
		normalizedType == "citext":
		return QuoteString(fmt.Sprintf("%v", value))

	// Binary
	case normalizedType == "bytea":
		if b, ok := value.([]byte); ok {
			return fmt.Sprintf("'\\x%x'", b)
		}
		return QuoteString(fmt.Sprintf("%v", value))

	// Network types
	case normalizedType == "inet" ||
		normalizedType == "cidr" ||
		normalizedType == "macaddr" ||
		normalizedType == "macaddr8":
		return QuoteString(fmt.Sprintf("%v", value))

	// Geometric types
	case normalizedType == "point" ||
		normalizedType == "line" ||
		normalizedType == "lseg" ||
		normalizedType == "box" ||
		normalizedType == "path" ||
		normalizedType == "polygon" ||
		normalizedType == "circle":
		return QuoteString(fmt.Sprintf("%v", value))

	// Other types (enums, custom types, etc.)
	default:
		return QuoteString(fmt.Sprintf("%v", value))
	}
}

// QuoteString properly escapes a string for PostgreSQL SQL literals.
func QuoteString(s string) string {
	// Check if we need E'' syntax for escape sequences
	needsEscape := strings.Contains(s, "\\")

	// Escape single quotes by doubling them
	escaped := strings.ReplaceAll(s, "'", "''")

	if needsEscape {
		// Use E'' syntax for strings containing backslashes
		return fmt.Sprintf("E'%s'", escaped)
	}
	return fmt.Sprintf("'%s'", escaped)
}

// quoteDollar uses PostgreSQL dollar-quoting for values that have complex escaping needs.
// It automatically generates a unique tag that doesn't appear in the value.
func quoteDollar(s string) string {
	tag := "q"
	for strings.Contains(s, "$"+tag+"$") {
		tag += "q"
	}
	return fmt.Sprintf("$%s$%s$%s$", tag, s, tag)
}

// SerializeRow converts a row of values to SQL literal strings.
func SerializeRow(values []interface{}, columns []ColumnInfo) []string {
	result := make([]string, len(values))
	for i, v := range values {
		var pgType string
		if i < len(columns) {
			pgType = columns[i].DataType
		}
		result[i] = SerializeValue(v, pgType)
	}
	return result
}
