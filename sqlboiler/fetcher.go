// Package sqlboiler provides adapters for integrating SQLBoiler with paging-go.
//
// This package provides a generic Fetcher[T] implementation that works with
// SQLBoiler-generated models, plus strategy-specific query builders for
// offset and cursor pagination.
//
// The design separates ORM integration (generic) from pagination strategy
// (specific), making it easy to:
//   1. Add new pagination strategies without changing the fetcher
//   2. Port to other ORMs (GORM, sqlc, etc.) by implementing Fetcher[T]
//
// Example usage:
//
//	// Create fetcher (ORM-specific, strategy-agnostic)
//	fetcher := sqlboiler.NewFetcher(
//	    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
//	        return models.Users(mods...).All(ctx, db)
//	    },
//	    func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
//	        return models.Users(mods...).Count(ctx, db)
//	    },
//	)
//
//	// Use with offset pagination
//	offsetPaginator := offset.NewPaginator(fetcher, ...)
//
//	// Or use with cursor pagination (Phase 2)
//	cursorPaginator := cursor.NewPaginator(fetcher, ...)
package sqlboiler

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
)

// QueryFunc executes a SQLBoiler query and returns results.
// This is ORM-specific but strategy-agnostic.
//
// Type parameter T is the SQLBoiler model type (e.g., *models.User).
type QueryFunc[T any] func(ctx context.Context, mods ...qm.QueryMod) ([]T, error)

// CountFunc executes a SQLBoiler count query.
// This is ORM-specific but strategy-agnostic.
type CountFunc func(ctx context.Context, mods ...qm.QueryMod) (int64, error)

// Fetcher implements paging.Fetcher[T] for SQLBoiler queries.
// It's generic and works with any pagination strategy by converting
// FetchParams into SQLBoiler query mods.
//
// The actual conversion logic is strategy-specific and provided by
// functions like OffsetToQueryMods() or CursorToQueryMods() (Phase 2).
type Fetcher[T any] struct {
	queryFunc   QueryFunc[T]
	countFunc   CountFunc
	queryModsFn func(paging.FetchParams) []qm.QueryMod
}

// NewFetcher creates a new SQLBoiler fetcher with a strategy-specific query builder.
//
// Parameters:
//   - queryFunc: Function that executes SQLBoiler queries with query mods
//   - countFunc: Function that counts total records with query mods
//   - queryModsFn: Strategy-specific function to convert FetchParams to QueryMods
//
// Example (offset pagination):
//
//	fetcher := sqlboiler.NewFetcher(
//	    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
//	        return models.Users(mods...).All(ctx, db)
//	    },
//	    func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
//	        return models.Users(mods...).Count(ctx, db)
//	    },
//	    sqlboiler.OffsetToQueryMods, // ← Strategy-specific!
//	)
func NewFetcher[T any](
	queryFunc QueryFunc[T],
	countFunc CountFunc,
	queryModsFn func(paging.FetchParams) []qm.QueryMod,
) paging.Fetcher[T] {
	return &Fetcher[T]{
		queryFunc:   queryFunc,
		countFunc:   countFunc,
		queryModsFn: queryModsFn,
	}
}

// Fetch retrieves items from the database using SQLBoiler query mods.
// The query mods are built using the strategy-specific queryModsFn.
func (f *Fetcher[T]) Fetch(ctx context.Context, params paging.FetchParams) ([]T, error) {
	mods := f.queryModsFn(params)
	return f.queryFunc(ctx, mods...)
}

// Count returns the total number of items matching the filters.
// Note: Filter support will be added in a future phase.
func (f *Fetcher[T]) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return f.countFunc(ctx)
}
