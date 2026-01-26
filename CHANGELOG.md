# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Breaking:** `db setup` no longer runs migrations or applies seeds. Use the new workflow:
  1. `seedup db setup` - creates database infrastructure only
  2. `seedup seed apply <name>` - runs initial migration + loads seed data
  3. `seedup migrate up` - runs remaining migrations
- **Breaking:** `seed apply` no longer runs remaining migrations after loading seed data. Run `migrate up` separately.
- **Breaking:** Removed `--seed-name` and `--skip-seed` flags from `db setup` command.
- **Breaking:** `DBSetupOptions` Go API no longer includes `MigrationsDir`, `SeedDir`, `SeedName`, or `SkipSeed` fields.

### Why This Change?

The previous behavior coupled database setup with migrations and seeds, which caused issues when later migrations modified column types that seed data referenced. The new decoupled workflow allows seeds to be created against a specific schema version and applied before running migrations that might be incompatible with the seed data format.

## [0.2.0] - 2026-01-23

### Added

- `seedup.Run()` and `seedup.RunArgs()` functions for embedding seedup in other binaries.
- `--schemas` flag for `seed create` to specify which schemas to include.
- `--all-schemas` flag for `seed create` to include all non-system schemas.

### Changed

- `seed create` now only dumps `public` schema by default (previously included all schemas).
- Improved error handling: no longer shows help text on command errors.

### Fixed

- Fixed dump file generation issues.

## [0.1.0] - 2026-01-18

### Added

- Go library API (`seedup.MigrateUp`, `seedup.SeedApply`, `seedup.DBSetup`, etc.).
- Single `load.sql` file format for seeds (replaces per-table CSV files).
- SQL-based seed format with batched INSERT statements.
- DBML schema documentation generation (`seedup dbml`).

### Changed

- Migrated from CSV to SQL format for seed files.
- Seeds now use a single `load.sql` file instead of multiple per-table files.
- Updated to Go 1.24.

### Fixed

- Prevented `pg_temp` schemas from leaking into load files.
- Fixed transaction handling in seed operations.

## [0.0.1] - 2026-01-09

### Added

- Initial release.
- `migrate` commands: `up`, `down`, `status`, `create`, `up-by-one`.
- `seed` commands: `apply`, `create`.
- `db` commands: `setup`, `drop`, `create`.
- `flatten` command to consolidate migrations.
- `check` command for CI validation of migration timestamps.
- PostgreSQL connection URL parsing and admin operations.
