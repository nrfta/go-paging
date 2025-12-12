package paging_test

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SeedUsers creates test users in the database and returns their IDs.
func SeedUsers(ctx context.Context, db *sql.DB, count int) ([]string, error) {
	userIDs := make([]string, count)

	for i := 0; i < count; i++ {
		id := uuid.New().String()
		age := 20 + (i % 60) // Ages between 20-79

		query := `
			INSERT INTO users (id, email, name, age, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`

		// Stagger created_at times to test ordering
		createdAt := time.Now().Add(-time.Duration(count-i) * time.Hour)

		_, err := db.ExecContext(ctx, query,
			id,
			fmt.Sprintf("user%d@example.com", i+1),
			fmt.Sprintf("User %d", i+1),
			age,
			i%3 != 0, // Most users active, some inactive
			createdAt,
			createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to seed user %d: %w", i, err)
		}

		userIDs[i] = id
	}

	return userIDs, nil
}

// SeedPosts creates test posts for the given users and returns their IDs.
func SeedPosts(ctx context.Context, db *sql.DB, userIDs []string, postsPerUser int) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("no user IDs provided")
	}

	postIDs := make([]string, 0, len(userIDs)*postsPerUser)

	for _, userID := range userIDs {
		for i := 0; i < postsPerUser; i++ {
			id := uuid.New().String()
			content := fmt.Sprintf("This is the content for post %s", id)

			query := `
				INSERT INTO posts (id, user_id, title, content, view_count, published_at, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`

			createdAt := time.Now().Add(-time.Duration(len(userIDs)*postsPerUser-len(postIDs)) * time.Hour)

			// Some posts published, some not (drafts)
			var publishedAt *time.Time
			if i%3 != 0 {
				pubTime := createdAt.Add(time.Hour)
				publishedAt = &pubTime
			}

			_, err := db.ExecContext(ctx, query,
				id,
				userID,
				fmt.Sprintf("Post %d by user %s", i+1, userID[:8]),
				content,
				i*10, // Varying view counts
				publishedAt,
				createdAt,
				createdAt,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to seed post for user %s: %w", userID, err)
			}

			postIDs = append(postIDs, id)
		}
	}

	return postIDs, nil
}

// CleanupTables truncates all test tables.
// Useful for cleanup between tests when sharing a database instance.
func CleanupTables(ctx context.Context, db *sql.DB) error {
	// Truncate in correct order (posts first due to FK constraint)
	tables := []string{"posts", "users"}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to truncate table %s: %w", table, err)
		}
	}

	return nil
}

// TrimToLimit returns items[:limit] if len(items) > limit, otherwise returns items unchanged.
// Used for N+1 pattern comparisons where we fetch LIMIT+1 but only want to compare LIMIT items.
func TrimToLimit[T any](items []T, limit int) []T {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

