// Package plancheck guards against query-plan regressions in database views.
//
// It enumerates every view in the target schemas, runs EXPLAIN (FORMAT JSON)
// against each, and compares the planner's estimated total cost to a committed
// baseline. A view fails when its cost exceeds max(baseline*factor, MaxCost),
// which catches both regressions of existing views and expensive new ones,
// while ignoring views that were already slow.
//
// Meaningful costs require production-scale planner statistics; apply a
// `seedup stats` dump to the (data-less) schema first, or pass StatsFile.
package plancheck

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/lucasefe/seedup/pkg/pgconn"
)

// Options configures a plan check.
type Options struct {
	Schemas          []string // Schemas to include (empty = all non-system)
	ExcludeViews     []string // View names to skip (unqualified)
	StatsFile        string   // Optional stats SQL to apply before planning
	BaselineFile     string   // Baseline JSON path (view -> cost)
	UpdateBaseline   bool     // Rewrite baseline instead of failing
	Analyze          bool     // ANALYZE before planning so costs are reproducible
	MaxCost          float64  // Absolute ceiling for new views (no baseline)
	RegressionFactor float64  // Allowed multiple of baseline cost
	MinDelta         float64  // Minimum absolute cost increase to count as a regression
}

// newViewBaseline marks a Result whose view has no baseline entry yet.
const newViewBaseline = -1

// Result is the outcome for a single view.
type Result struct {
	View     string
	Cost     float64
	Baseline float64 // newViewBaseline when the view is new
	Failed   bool
}

// Checker runs plan checks against a database.
type Checker struct{}

// New creates a new Checker.
func New() *Checker { return &Checker{} }

// Check enumerates views, plans each, and compares to the baseline.
// It returns an error when any view fails (unless UpdateBaseline is set).
func (c *Checker) Check(ctx context.Context, dbURL string, opts Options) error {
	db, err := pgconn.Open(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if opts.StatsFile != "" {
		if err := applyFile(ctx, db, opts.StatsFile); err != nil {
			return fmt.Errorf("applying stats: %w", err)
		}
	} else if opts.Analyze {
		// Settle planner statistics so identical schemas plan identically:
		// otherwise autovacuum timing drifts costs between the baseline run
		// and the check. On seed-sized tables ANALYZE reads every row, so the
		// resulting stats (and costs) are deterministic. Skipped when stats are
		// injected, since ANALYZE would overwrite them.
		if _, err := db.ExecContext(ctx, "ANALYZE"); err != nil {
			return fmt.Errorf("analyzing: %w", err)
		}
	}

	baseline, err := loadBaseline(opts.BaselineFile)
	if err != nil {
		return err
	}

	views, err := c.views(ctx, db, opts.Schemas, opts.ExcludeViews)
	if err != nil {
		return err
	}

	results := make([]Result, 0, len(views))
	currentCosts := map[string]float64{}
	for _, view := range views {
		result, err := c.evaluateView(ctx, db, view, baseline, opts)
		if err != nil {
			return err
		}
		currentCosts[view] = result.Cost
		results = append(results, result)
	}

	printResults(results)

	if opts.UpdateBaseline {
		if err := writeBaseline(opts.BaselineFile, currentCosts); err != nil {
			return err
		}
		fmt.Printf("Wrote baseline for %d views to %s\n", len(currentCosts), opts.BaselineFile)
		return nil
	}

	return regressionError(results)
}

// evaluateView plans a single view and decides whether its cost breaches the
// budget relative to the baseline.
func (c *Checker) evaluateView(ctx context.Context, db *sql.DB, view string, baseline map[string]float64, opts Options) (Result, error) {
	cost, err := planCost(ctx, db, view)
	if err != nil {
		return Result{}, fmt.Errorf("explaining %s: %w", view, err)
	}

	baselineCost, hasBaseline := baseline[view]
	result := Result{View: view, Cost: cost, Baseline: newViewBaseline}
	if hasBaseline {
		result.Baseline = baselineCost
	}
	result.Failed = !opts.UpdateBaseline &&
		failsBudget(cost, baselineCost, hasBaseline, opts.MaxCost, opts.RegressionFactor, opts.MinDelta)
	return result, nil
}

// regressionError builds the failure returned when any view breached the budget.
func regressionError(results []Result) error {
	var regressed []string
	for _, result := range results {
		if result.Failed {
			regressed = append(regressed, result.View)
		}
	}
	if len(regressed) == 0 {
		return nil
	}
	return fmt.Errorf("%d view(s) exceeded the plan-cost budget: %s\n"+
		"If the change is intentional, refresh the baseline with --update-baseline.",
		len(regressed), strings.Join(regressed, ", "))
}

func (c *Checker) views(ctx context.Context, db *sql.DB, schemas, exclude []string) ([]string, error) {
	query := `
SELECT schemaname, viewname FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')`
	var args []any
	if len(schemas) > 0 {
		query += " AND schemaname = ANY($1)"
		args = append(args, pq.Array(schemas))
	}
	query += `
UNION ALL
SELECT schemaname, matviewname FROM pg_matviews
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')`
	if len(schemas) > 0 {
		query += " AND schemaname = ANY($1)"
	}
	query += " ORDER BY 1, 2"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing views: %w", err)
	}
	defer rows.Close()

	skip := map[string]bool{}
	for _, v := range exclude {
		skip[v] = true
	}

	var out []string
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, err
		}
		if skip[name] {
			continue
		}
		out = append(out, schema+"."+name)
	}
	return out, rows.Err()
}

func planCost(ctx context.Context, db *sql.DB, qualified string) (float64, error) {
	schema, name, _ := strings.Cut(qualified, ".")
	q := fmt.Sprintf(`EXPLAIN (FORMAT JSON) SELECT * FROM %s.%s`, pq.QuoteIdentifier(schema), pq.QuoteIdentifier(name))

	var raw []byte
	if err := db.QueryRowContext(ctx, q).Scan(&raw); err != nil {
		return 0, err
	}
	return parsePlanCost(raw)
}

// failsBudget reports whether a view's cost breaches the budget. A view with a
// baseline fails when it grows past baseline*factor by more than minDelta (the
// minDelta guard keeps tiny-cost views from flapping on planner noise). A view
// without a baseline (new) fails only when it exceeds the absolute maxCost
// ceiling, so ordinary new views pass and get recorded on the next baseline.
func failsBudget(cost, baseline float64, hasBaseline bool, maxCost, factor, minDelta float64) bool {
	if !hasBaseline {
		return cost > maxCost
	}
	return cost > baseline*factor && cost-baseline > minDelta
}

func parsePlanCost(raw []byte) (float64, error) {
	var plans []struct {
		Plan struct {
			TotalCost float64 `json:"Total Cost"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		return 0, fmt.Errorf("parsing plan JSON: %w", err)
	}
	if len(plans) == 0 {
		return 0, fmt.Errorf("empty plan")
	}
	return plans[0].Plan.TotalCost, nil
}

func loadBaseline(path string) (map[string]float64, error) {
	if path == "" {
		return map[string]float64{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]float64{}, nil
		}
		return nil, fmt.Errorf("reading baseline: %w", err)
	}
	m := map[string]float64{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing baseline %s: %w", path, err)
	}
	return m, nil
}

func writeBaseline(path string, costs map[string]float64) error {
	if path == "" {
		return fmt.Errorf("baseline file required to update baseline")
	}
	data, err := json.MarshalIndent(costs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func applyFile(ctx context.Context, db *sql.DB, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(content))
	return err
}

func printResults(results []Result) {
	sort.Slice(results, func(i, j int) bool { return results[i].Cost > results[j].Cost })
	for _, r := range results {
		status := "ok"
		if r.Failed {
			status = "FAIL"
		}
		base := "new"
		if r.Baseline != newViewBaseline {
			base = fmt.Sprintf("%.0f", r.Baseline)
		}
		fmt.Printf("  [%-4s] %-45s cost=%.0f baseline=%s\n", status, r.View, r.Cost, base)
	}
}
