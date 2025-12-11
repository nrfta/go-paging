package sqlboiler_test

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/sqlboiler"
)

// This example demonstrates how to create a SQLBoiler fetcher for offset pagination.
// The same pattern works for any pagination strategy by changing the query builder function.
func ExampleNewFetcher_offset() {
	// Mock database models
	type User struct {
		ID   string
		Name string
	}

	// Your SQLBoiler query functions
	queryFunc := func(ctx context.Context, mods ...qm.QueryMod) ([]*User, error) {
		// In real code: return models.Users(mods...).All(ctx, db)
		return []*User{{ID: "1", Name: "Alice"}}, nil
	}

	countFunc := func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
		// In real code: return models.Users(mods...).Count(ctx, db)
		return 100, nil
	}

	// Create fetcher with OFFSET strategy
	fetcher := sqlboiler.NewFetcher(
		queryFunc,
		countFunc,
		sqlboiler.OffsetToQueryMods, // ← Offset-specific query builder
	)

	// Use with any paginator that needs a fetcher
	ctx := context.Background()
	results, _ := fetcher.Fetch(ctx, paging.FetchParams{
		Limit:  10,
		Offset: 20,
		OrderBy: []paging.OrderBy{
			{Column: "created_at", Desc: true},
		},
	})

	println(len(results)) // 1
}

// This example shows how you would use a different strategy (cursor pagination in Phase 2).
// The Fetcher[T] stays the same, only the query builder changes!
func ExampleNewFetcher_cursor() {
	type User struct {
		ID   string
		Name string
	}

	queryFunc := func(ctx context.Context, mods ...qm.QueryMod) ([]*User, error) {
		return []*User{{ID: "1", Name: "Alice"}}, nil
	}

	countFunc := func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
		return 100, nil
	}

	// Phase 2: Create fetcher with CURSOR strategy
	// (CursorToQueryMods doesn't exist yet, this is for illustration)
	_ = sqlboiler.NewFetcher(
		queryFunc,
		countFunc,
		// sqlboiler.CursorToQueryMods, // ← Cursor-specific query builder (Phase 2)
		sqlboiler.OffsetToQueryMods, // Using offset for now
	)

	// The beauty: Same Fetcher[T] type, different strategy!
	// Easy to add new strategies without changing the fetcher.
}

// This example shows how you could create an adapter for a different ORM.
// The pattern is the same: implement Fetcher[T] interface.
func ExampleNewFetcher_differentORM() {
	// Example: Hypothetical GORM adapter (not implemented)
	type User struct {
		ID   string
		Name string
	}

	// You would create a similar pattern for GORM:
	// gorm/
	// ├── fetcher.go          # Generic GORM Fetcher[T]
	// ├── offset.go           # GORM-specific offset query builder
	// └── cursor.go           # GORM-specific cursor query builder

	// Usage would be identical:
	// fetcher := gorm.NewFetcher(
	//     queryFunc,
	//     countFunc,
	//     gorm.OffsetToQueryMods,
	// )

	// The pagination logic (offset.Paginator, cursor.Paginator) works with ANY fetcher!
}
