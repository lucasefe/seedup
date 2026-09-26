package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasefe/seedup/pkg/stats"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var (
		output        string
		schemas       string
		excludeTables string
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Dump sanitized planner statistics",
		Long: `Dump sanitized PostgreSQL planner statistics as SQL.

The output contains pg_restore_relation_stats / pg_restore_attribute_stats calls
(PostgreSQL 18+) carrying only cardinalities: reltuples, relpages, n_distinct,
null_frac and avg_width. It never reads most_common_vals or histogram_bounds, so
it holds no real column values and is safe to commit.

Apply the result to a migrated, data-less schema to plan queries against
production-scale cardinalities (see 'seedup plancheck').

Examples:
  seedup stats -o stats.sql --all-schemas
  seedup stats --schemas public,reports -o stats.sql`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			opts := stats.Options{Output: output}
			if schemas != "" {
				opts.Schemas = strings.Split(schemas, ",")
			}
			if excludeTables != "" {
				opts.ExcludeTables = strings.Split(excludeTables, ",")
			}

			return stats.New().Dump(context.Background(), dbURL, opts)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&schemas, "schemas", "", "Comma-separated schemas to include")
	cmd.Flags().StringVar(&excludeTables, "exclude-tables", "", "Comma-separated tables to exclude")
	// --all-schemas is accepted for symmetry with `dbml`; it is the default.
	cmd.Flags().BoolP("all-schemas", "a", false, "Include all non-system schemas (default)")

	return cmd
}
