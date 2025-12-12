# Test Models

This directory contains SQLBoiler-generated models for integration testing.

## Files

- **`schema.sql`** - PostgreSQL schema defining test tables (users, posts)
- **`sqlboiler.toml`** - SQLBoiler configuration for model generation
- **`Dockerfile`** - Docker image with SQLBoiler for model generation
- **`*.go`** - Generated SQLBoiler models (do not edit manually)

## Regenerating Models

If you modify `schema.sql`, regenerate the models:

### Using Docker (Recommended)

```bash
# From repository root
docker build -t go-paging-sqlboiler -f tests/models/Dockerfile .

# Run SQLBoiler to generate models
docker run --rm \
  --network host \
  -v "$(pwd):/workspace" \
  go-paging-sqlboiler \
  "sqlboiler psql -c tests/models/sqlboiler.toml"
```

### Using Local SQLBoiler

```bash
# Install SQLBoiler
go install github.com/aarondl/sqlboiler/v4@latest
go install github.com/aarondl/sqlboiler/v4/drivers/sqlboiler-psql@latest

# Start PostgreSQL (integration tests use testcontainers, so this is just for generation)
# You'll need a local PostgreSQL instance with the schema loaded

# Generate models
sqlboiler psql -c tests/models/sqlboiler.toml
```

## Schema Overview

### `users` Table

- `id` (UUID, primary key)
- `name` (TEXT)
- `email` (TEXT)
- `created_at` (TIMESTAMP)
- `updated_at` (TIMESTAMP)

### `posts` Table

- `id` (UUID, primary key)
- `user_id` (UUID, foreign key → users)
- `title` (TEXT)
- `content` (TEXT)
- `published_at` (TIMESTAMP, nullable)
- `created_at` (TIMESTAMP)
- `updated_at` (TIMESTAMP)

## Notes

- Models are regenerated when schema changes
- The `sqlboiler.toml` config outputs to this directory
- Integration tests use testcontainers, so no local PostgreSQL required for testing
- The schema is applied automatically by integration tests via migrations
