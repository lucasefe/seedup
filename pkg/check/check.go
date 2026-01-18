package check

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucasefe/seedup/pkg/executor"
)

// Checker validates migration file timestamps
type Checker struct {
	exec executor.Executor
}

// New creates a new Checker with the given executor
func New(exec executor.Executor) *Checker {
	return &Checker{exec: exec}
}

// Check validates that new migrations have the latest timestamps
// This prevents merge conflicts when multiple developers add migrations
func (c *Checker) Check(ctx context.Context, migrationsDir, baseBranch string) error {
	// Fetch the base branch if in CI
	if err := c.fetchBaseBranch(ctx, baseBranch); err != nil {
		// Ignore fetch errors - may already have the branch
		_ = err
	}

	// Get all migration files sorted by name (descending)
	allMigrations, err := c.getAllMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("getting migrations: %w", err)
	}

	// Get new migrations added in this branch
	newMigrations, err := c.getNewMigrations(ctx, migrationsDir, baseBranch)
	if err != nil {
		return fmt.Errorf("getting new migrations: %w", err)
	}

	if len(newMigrations) == 0 {
		fmt.Println("No new migrations added")
		return nil
	}

	// The N newest migrations should be exactly the new migrations
	expectedLatest := allMigrations[:len(newMigrations)]

	// Sort both slices for comparison
	sort.Sort(sort.Reverse(sort.StringSlice(expectedLatest)))
	sort.Sort(sort.Reverse(sort.StringSlice(newMigrations)))

	if !slicesEqual(expectedLatest, newMigrations) {
		return c.formatError(newMigrations, migrationsDir)
	}

	fmt.Println("New migrations have the latest timestamps, all is good")
	return nil
}

func (c *Checker) fetchBaseBranch(ctx context.Context, baseBranch string) error {
	return c.exec.Run(ctx, "git", "fetch", "origin", baseBranch)
}

func (c *Checker) getAllMigrations(dir string) ([]string, error) {
	pattern := filepath.Join(dir, "*.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort descending by filename
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

func (c *Checker) getNewMigrations(ctx context.Context, dir, baseBranch string) ([]string, error) {
	pattern := filepath.Join(dir, "*.sql")
	output, err := c.exec.RunWithOutput(ctx, "git", "diff", "--name-only", "--diff-filter=A",
		"origin/"+baseBranch, "--", pattern)
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			migrations = append(migrations, line)
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(migrations)))
	return migrations, nil
}

func (c *Checker) formatError(newMigrations []string, dir string) error {
	var msg strings.Builder
	msg.WriteString("Error: New migrations must have the latest timestamps\n\n")
	msg.WriteString("To fix:\n\n")

	for _, file := range newMigrations {
		filename := filepath.Base(file)
		parts := strings.SplitN(filename, "_", 2)
		if len(parts) != 2 {
			continue
		}
		timestamp := parts[0]
		rest := parts[1]

		msg.WriteString(fmt.Sprintf("  $ git mv %s/{%s,$(date -u +%%Y%%m%%d%%H%%M%%S)}_%s\n",
			dir, timestamp, rest))
	}

	return errors.New(msg.String())
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if filepath.Base(a[i]) != filepath.Base(b[i]) {
			return false
		}
	}
	return true
}

// IsGitRepo checks if the current directory is inside a git repository
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	return cmd.Run() == nil
}
