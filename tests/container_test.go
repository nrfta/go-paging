package paging_test

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Container represents a running PostgreSQL testcontainer.
// It provides a fully configured PostgreSQL instance with tables and test data.
type Container struct {
	Container *postgres.PostgresContainer
	DB        *sql.DB
	ConnStr   string
}

// SetupPostgres starts a PostgreSQL container with initialized tables.
func SetupPostgres(ctx context.Context) (*Container, error) {
	// Start PostgreSQL container
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}

	// Get connection string
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create tables
	if err := createTables(ctx, db); err != nil {
		db.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &Container{
		Container: pgContainer,
		DB:        db,
		ConnStr:   connStr,
	}, nil
}

// Terminate stops and removes the PostgreSQL container.
func (c *Container) Terminate(ctx context.Context) error {
	if c.DB != nil {
		c.DB.Close()
	}
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// createTables creates the test schema.
// Tables are designed to test pagination features:
// - users: Basic pagination with various data types
// - posts: Sorting, filtering, and relationships
func createTables(ctx context.Context, db *sql.DB) error {
	schema := `
		-- Users table for basic pagination tests
		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			age INTEGER,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		-- Posts table for testing sorting and relationships
		CREATE TABLE posts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title VARCHAR(500) NOT NULL,
			content TEXT,
			view_count INTEGER DEFAULT 0,
			published_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		-- Indexes for efficient pagination queries
		CREATE INDEX idx_users_created_at ON users(created_at DESC, id DESC);
		CREATE INDEX idx_users_email ON users(email);
		CREATE INDEX idx_posts_user_id ON posts(user_id);
		CREATE INDEX idx_posts_created_at ON posts(created_at DESC, id DESC);
		CREATE INDEX idx_posts_published_at ON posts(published_at DESC NULLS LAST, id DESC);
	`

	_, err := db.ExecContext(ctx, schema)
	return err
}
