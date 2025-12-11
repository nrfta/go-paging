package models

import (
	"context"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/aarondl/sqlboiler/v4/queries/qm"
)

// Post is an object representing the database table.
type Post struct {
	ID          string     `boil:"id" json:"id"`
	UserID      string     `boil:"user_id" json:"user_id"`
	Title       string     `boil:"title" json:"title"`
	Content     string     `boil:"content" json:"content"`
	ViewCount   int        `boil:"view_count" json:"view_count"`
	PublishedAt *time.Time `boil:"published_at" json:"published_at,omitempty"`
	CreatedAt   time.Time  `boil:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `boil:"updated_at" json:"updated_at"`
}

const (
	postTable   = "posts"
	postColumns = "id, user_id, title, content, view_count, published_at, created_at, updated_at"
)

type postQuery struct {
	mods []qm.QueryMod
}

// Posts returns a new query against the posts table.
func Posts(mods ...qm.QueryMod) postQuery {
	return postQuery{mods: mods}
}

// All returns all Post records from the query.
func (q postQuery) All(ctx context.Context, exec boil.ContextExecutor) ([]*Post, error) {
	params := ParseQueryMods(q.mods)
	query := BuildSelectQuery(postTable, postColumns, params)

	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*Post
	for rows.Next() {
		post := &Post{}
		err := rows.Scan(&post.ID, &post.UserID, &post.Title, &post.Content, &post.ViewCount, &post.PublishedAt, &post.CreatedAt, &post.UpdatedAt)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, rows.Err()
}

// Count returns the count of all Post records in the query.
func (q postQuery) Count(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	params := ParseQueryMods(q.mods)
	query := BuildCountQuery(postTable, params)

	var count int64
	err := exec.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}
