.PHONY: run reset db-create db-drop fresh seed-create

DBNAME = seedup-usage-example
SEEDUP = go run ../cmd/seedup/main.go
DB_URL = postgres://$(shell whoami)@localhost/$(DBNAME)?sslmode=disable

# Reset migrations folder and run the example
run: reset
	go run usage.go

# Reset migrations and seed folders to clean state
reset:
	rm -rf migrations/*
	cp migrations_source/*.sql migrations/
	rm -f seed/dev/load.sql

# Create the database
db-create:
	createdb $(DBNAME) 2>/dev/null || true

# Drop the database
db-drop:
	dropdb $(DBNAME) 2>/dev/null || true

# Full reset: drop + create db + run
fresh: db-drop db-create run

# Create seed data from the current database state
# 1. Drops and recreates DB for clean state
# 2. Resets migrations to source
# 3. Runs migrations (populates DB with test data)
# 4. Extracts data to seed/dev/load.sql using the query in seed/dev/dump.sql
seed-create: db-drop db-create reset
	$(SEEDUP) migrate up -d "$(DB_URL)"
	$(SEEDUP) seed create dev -d "$(DB_URL)"
	@echo "Seed created: seed/dev/load.sql"
