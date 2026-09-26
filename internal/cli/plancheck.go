package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucasefe/seedup/pkg/plancheck"
	"github.com/spf13/cobra"
)

func newPlanCheckCmd() *cobra.Command {
	var (
		schemas          string
		excludeViews     string
		statsFile        string
		baselineFile     string
		updateBaseline   bool
		noAnalyze        bool
		maxCost          float64
		regressionFactor float64
		minDelta         float64
	)

	cmd := &cobra.Command{
		Use:   "plancheck",
		Short: "Detect query-plan regressions in views",
		Long: `Run EXPLAIN against every view and fail when a plan's estimated cost
regresses beyond the committed baseline.

A view fails when its cost exceeds max(baseline * regression-factor, max-cost),
catching regressions of existing views and expensive new ones while ignoring
views that were already slow.

Costs are only meaningful with production-scale planner statistics: apply a
'seedup stats' dump to the migrated (data-less) schema, or pass --stats.

Examples:
  seedup plancheck --stats stats.sql --baseline view_baseline.json
  seedup plancheck --baseline view_baseline.json --update-baseline`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dbURL := getDatabaseURL()
			if dbURL == "" {
				return fmt.Errorf("database URL required (use -d flag or DATABASE_URL env)")
			}

			opts := plancheck.Options{
				StatsFile:        statsFile,
				BaselineFile:     baselineFile,
				UpdateBaseline:   updateBaseline,
				Analyze:          !noAnalyze,
				MaxCost:          maxCost,
				RegressionFactor: regressionFactor,
				MinDelta:         minDelta,
			}
			if schemas != "" {
				opts.Schemas = strings.Split(schemas, ",")
			}
			if excludeViews != "" {
				opts.ExcludeViews = strings.Split(excludeViews, ",")
			}

			return plancheck.New().Check(context.Background(), dbURL, opts)
		},
	}

	cmd.Flags().StringVar(&schemas, "schemas", "", "Comma-separated schemas to include")
	cmd.Flags().StringVar(&excludeViews, "exclude-views", "", "Comma-separated view names to skip")
	cmd.Flags().StringVar(&statsFile, "stats", "", "Statistics SQL file to apply before planning")
	cmd.Flags().StringVar(&baselineFile, "baseline", "", "Baseline JSON file (view -> cost)")
	cmd.Flags().BoolVar(&updateBaseline, "update-baseline", false, "Rewrite the baseline instead of failing")
	cmd.Flags().BoolVar(&noAnalyze, "no-analyze", false, "Skip ANALYZE before planning (costs may drift with autovacuum)")
	cmd.Flags().Float64Var(&maxCost, "max-cost", 100000, "Absolute cost ceiling for new views (no baseline)")
	cmd.Flags().Float64Var(&regressionFactor, "regression-factor", 1.5, "Allowed multiple of a view's baseline cost")
	cmd.Flags().Float64Var(&minDelta, "min-delta", 100, "Minimum absolute cost increase to count as a regression")

	return cmd
}
