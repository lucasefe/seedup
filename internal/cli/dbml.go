package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tmwinc/seedup/pkg/dbml"
)

func newDBMLCmd() *cobra.Command {
	var (
		output        string
		schemas       string
		excludeTables string
		allSchemas    bool
	)

	cmd := &cobra.Command{
		Use:   "dbml",
		Short: "Generate DBML from database schema",
		Long: `Generate DBML (Database Markup Language) documentation from the database schema.

DBML files can be visualized at https://dbdiagram.io

Examples:
  seedup dbml                              # Output to stdout
  seedup dbml -o schema.dbml               # Output to file
  seedup dbml --all-schemas                # Include all schemas
  seedup dbml --exclude-tables goose_db_version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			gen := dbml.New()

			opts := dbml.Options{
				Output:     output,
				AllSchemas: allSchemas,
			}
			if schemas != "" {
				opts.Schemas = strings.Split(schemas, ",")
			}
			if excludeTables != "" {
				opts.ExcludeTables = strings.Split(excludeTables, ",")
			}

			return gen.Generate(context.Background(), dbURL, opts)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&schemas, "schemas", "", "Comma-separated schemas to include")
	cmd.Flags().StringVar(&excludeTables, "exclude-tables", "", "Comma-separated tables to exclude")
	cmd.Flags().BoolVarP(&allSchemas, "all-schemas", "a", false, "Include all non-system schemas")

	return cmd
}
