# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
# Build the CLI
go build ./cmd/seedup

# Build all packages
go build ./...

# Run tests
go test ./...

# Install locally
go install ./cmd/seedup

# Run the CLI directly
go run ./cmd/seedup <command>
```

## Architecture

seedup is a PostgreSQL migration and seed data management tool, usable as both a CLI and Go library.

### Dual Interface Pattern

The codebase exposes functionality through two interfaces that should stay in sync:

1. **CLI** (`internal/cli/`) - Cobra commands that parse flags and call into `pkg/`
2. **Go Library** (`seedup.go`) - Public API that wraps `pkg/` for programmatic use

When adding features, implement in `pkg/`, expose in `seedup.go`, and wire up CLI flags in `internal/cli/`. CLI flags should mirror Go API options.

### Package Structure

- `seedup.go` - Public Go API (re-exports from pkg/ with user-friendly option structs)
- `cmd/seedup/` - CLI entry point
- `internal/cli/` - CLI command implementations using Cobra
- `pkg/migrate/` - Migration operations (wraps goose)
- `pkg/seed/` - Seed data creation and application
- `pkg/db/` - Database lifecycle (create, drop, setup)
- `pkg/dbml/` - DBML schema documentation generation
- `pkg/pgconn/` - PostgreSQL connection utilities and schema introspection
- `pkg/check/` - CI validation for migration timestamps
- `pkg/executor/` - Shell command execution (used by check)

### Seed Data Flow

**Creating seeds** (`seed create`):
1. Creates temp tables matching each real table (`pg_temp."seed.<schema>.<table>"`)
2. Executes user's `dump.sql` which populates temp tables with desired data
3. Exports temp tables to `load.sql` as batched INSERT statements

**Applying seeds** (`seed apply`):
1. Runs initial migration
2. Loads `load.sql` into database
3. Runs remaining migrations

### Configuration

Environment variables with CLI flag overrides:
- `DATABASE_URL` / `-d` - PostgreSQL connection URL
- `MIGRATIONS_DIR` / `-m` - Migrations directory (default: `./migrations`)
- `SEED_DIR` - Seed data root directory (default: `./seed`)

## Documentation Updates

When implementing changes that affect user-facing functionality, update:

1. **README.md** - CLI command examples, Go API examples, usage patterns
2. **seedup.go** - Public API docstrings and option struct comments
