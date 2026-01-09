# seedup

A CLI tool for managing PostgreSQL database migrations and seed data. Wraps [goose](https://github.com/pressly/goose) for migrations and provides utilities for creating and applying seed data.

## Installation

```bash
go install github.com/tmwinc/seedup/cmd/seedup@latest
```

## Requirements

The following tools must be installed and available in your PATH:

### Install Dependencies

```bash
# goose - Database migrations
go install github.com/pressly/goose/v3/cmd/goose@latest

# dbml - Schema documentation generator (optional, for dbml command)
go install github.com/lucasefe/dbml@latest
```

### System Requirements

- `psql` / `pg_dump` - PostgreSQL client tools (install via `brew install postgresql` or your package manager)
- `git` - For the check command (CI validation)

## Quick Start

```bash
# Set your database URL
export DATABASE_URL="postgres://user:pass@localhost/mydb"

# Create your first migration
seedup migrate create create_users_table

# Edit the migration file, then run it
seedup migrate up

# Check migration status
seedup migrate status
```

## Integrating seedup into Your Project

### 1. Project Structure

Set up your project with the following structure:

```
your-project/
├── migrations/           # Migration files go here
│   └── 20240101120000_initial.sql
├── seed/                 # Seed data root directory
│   ├── dev.sql           # SQL to select seed data for "dev" seed set
│   └── dev/              # Seed data CSV files for "dev" seed set
│       ├── public.users.csv
│       └── public.accounts.csv
├── Makefile              # Optional: wrap seedup commands
└── ...
```

### 2. Environment Variables

Configure seedup using environment variables (12-factor style):

```bash
# Required
export DATABASE_URL="postgres://user:pass@localhost/mydb"

# Optional (with defaults)
export MIGRATIONS_DIR="./migrations"    # default: ./migrations
export SEED_DIR="./seed"                # default: ./seed
```

### 3. Makefile Integration

Add these targets to your `Makefile`:

```makefile
# Database migrations
.PHONY: migrate migrate-down migrate-status migrate-create

migrate:
	seedup migrate up

migrate-down:
	seedup migrate down

migrate-status:
	seedup migrate status

migrate-create:
	@read -p "Migration name: " name; \
	seedup migrate create $$name

# Seed data (use "dev" as the seed set name)
.PHONY: seed seed-create

seed:
	seedup seed apply dev

seed-create:
	seedup seed create dev -d "$$PROD_DATABASE_URL"

# Database setup
.PHONY: db-setup db-drop

db-setup:
	seedup db setup --force --seed-name dev

db-drop:
	seedup db drop --force

# CI checks
.PHONY: check-migrations

check-migrations:
	seedup check --base-branch main
```

### 4. CI/CD Integration

Add migration validation to your CI pipeline:

```yaml
# .github/workflows/ci.yml
jobs:
  check-migrations:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Required for branch comparison

      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install seedup
        run: go install github.com/tmwinc/seedup/cmd/seedup@latest

      - name: Check migration timestamps
        run: seedup check --base-branch ${{ github.base_ref || 'main' }}
```

## Commands

### migrate

Run database migrations using goose.

```bash
# Run all pending migrations
seedup migrate up

# Run a single migration
seedup migrate up-by-one

# Rollback the last migration
seedup migrate down

# Show migration status
seedup migrate status

# Create a new migration file
seedup migrate create add_users_table
# Creates: migrations/20240101120000_add_users_table.sql
```

### seed apply

Apply seed data to your local database. This is useful for setting up development environments.

```bash
# Apply the "dev" seed set
seedup seed apply dev

# Apply to a specific database
seedup seed apply dev -d "$DATABASE_URL"
```

The apply process:
1. Runs the initial migration (first migration file)
2. Loads all CSV files from `seed/<name>/`
3. Runs remaining migrations

### seed create

Create seed data from a database (typically production). This extracts data based on your query file.

```bash
# Create "dev" seed set from production
seedup seed create dev -d "$PROD_DATABASE_URL"

# Dry run (preview without modifying files)
seedup seed create dev -d "$PROD_DATABASE_URL" --dry-run
```

The create process:
1. Reads the query file at `seed/<name>.sql`
2. Executes queries against the source database
3. Exports results to CSV files in `seed/<name>/`
4. Flattens all migrations into a single initial migration

### flatten

Consolidate all migrations into a single initial migration. Useful for cleaning up migration history.

```bash
seedup flatten -d "$PROD_DATABASE_URL"
```

### check

Validate that new migrations have the latest timestamps. This prevents merge conflicts when multiple developers add migrations.

```bash
seedup check --base-branch main
```

If validation fails, seedup provides fix commands:

```
Error: New migrations must have the latest timestamps

To fix:

  $ git mv migrations/{20240101120000,$(date -u +%Y%m%d%H%M%S)}_add_users.sql
```

### db

Database lifecycle management commands for setting up and tearing down databases.

```bash
# Full setup: drop + create user + create db + permissions + migrate + seed
seedup db setup --seed-name dev

# Skip confirmation prompt (for CI/automation)
seedup db setup --force --seed-name dev

# Skip seeding (only create db and run migrations)
seedup db setup --skip-seed

# Drop the database
seedup db drop

# Drop without confirmation
seedup db drop --force

# Create the database (if it doesn't exist)
seedup db create
```

The `db setup` command performs:
1. Creates the database user (extracted from DATABASE_URL) if it doesn't exist
2. Drops the database if it exists
3. Creates the database
4. Sets up permissions (grants all privileges, sets owner)
5. Runs all migrations
6. Applies seed data from `seed/<name>/` (if `--seed-name` provided)

The database name, user, and password are all extracted from the DATABASE_URL.

### dbml

Generate DBML (Database Markup Language) documentation from your database schema.

```bash
# Generate DBML to stdout
seedup dbml

# Generate to file
seedup dbml -o schema.dbml

# Include all schemas (not just public)
seedup dbml --all-schemas -o schema.dbml

# Exclude specific tables
seedup dbml --exclude-tables goose_db_version
```

DBML files can be used with [dbdiagram.io](https://dbdiagram.io) to visualize your database schema.

## Writing Migrations

Migration files use the standard goose format:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd
```

## Writing Seed Query Files

The seed query file (e.g., `seed/dev.sql`) defines which data to extract from your source database. When you run `seedup seed create dev`, it:

1. Creates temporary tables for each table in the database
2. Runs your query file to populate those temp tables
3. Exports the temp tables to CSV files in `seed/dev/`

The query file should contain INSERT statements that select data FROM your real tables INTO the corresponding temp tables. Each temp table is named `pg_temp."seed.<schema>.<table>"`.

Example `seed/dev.sql`:

```sql
-- Select recent users for development
INSERT INTO pg_temp."seed.public.users" (id, name, email, created_at)
SELECT id, name, email, created_at
FROM public.users
WHERE created_at > NOW() - INTERVAL '30 days'
LIMIT 100;

-- Select accounts for those users
INSERT INTO pg_temp."seed.public.accounts" (id, user_id, name, balance)
SELECT a.id, a.user_id, a.name, a.balance
FROM public.accounts a
WHERE a.user_id IN (SELECT id FROM pg_temp."seed.public.users");

-- Select related data
INSERT INTO pg_temp."seed.public.transactions" (id, account_id, amount, created_at)
SELECT t.id, t.account_id, t.amount, t.created_at
FROM public.transactions t
WHERE t.account_id IN (SELECT id FROM pg_temp."seed.public.accounts")
LIMIT 1000;
```

## CLI Reference

### Global Flags

```
-d, --database-url string     Database URL (overrides DATABASE_URL env)
-m, --migrations-dir string   Migrations directory (overrides MIGRATIONS_DIR env)
-v, --verbose                 Verbose output
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection URL | required |
| `MIGRATIONS_DIR` | Path to migrations directory | `./migrations` |
| `SEED_DIR` | Path to seed data root directory | `./seed` |

## Examples

### Full Development Setup

```bash
# Clone your project
git clone https://github.com/yourorg/yourproject
cd yourproject

# Configure environment
export DATABASE_URL="postgres://user:pass@localhost/myproject_dev"

# Full database setup (creates db, user, runs migrations, seeds)
seedup db setup --seed-name dev

# Your database is now ready for development!
```

### Creating New Seed Data

```bash
# Connect to production (read-only)
export PROD_DATABASE_URL="postgres://readonly:pass@prod-host/myproject"

# Create "dev" seed set from production
seedup seed create dev -d "$PROD_DATABASE_URL"

# Review and commit the changes
git add migrations/ seed/
git commit -m "Update seed data"
```

### Adding a New Migration

```bash
# Create migration
seedup migrate create add_orders_table

# Edit the file
vim migrations/20240101120000_add_orders_table.sql

# Run it
seedup migrate up

# Commit
git add migrations/
git commit -m "Add orders table"
```

## License

MIT
