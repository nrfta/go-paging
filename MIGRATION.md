# Migration Guide: v0.3.0 to v2.0

This guide helps you migrate from paging-go v0.3.0 to v2.0.

## Overview

v2.0 introduces major changes to the API:

1. **Modular Package Structure** - Strategies moved to separate packages (`offset/`, `cursor/`, `quotafill/`)
2. **Fetcher Pattern** - All strategies now take `Fetcher[T]` as their first parameter
3. **Unified Paginator Interface** - All strategies implement the same `Paginator[T].Paginate()` method
4. **Functional Options** - Page size limits moved from constructors to per-request options
5. **Simplified BuildConnection** - Connection builders work with `*Page[T]` result type
6. **Consistent Metadata** - All strategies return rich metadata for observability

## Breaking Changes Summary

| Change | Old (v0.3.0) | New (v2.0) |
|--------|-----------|-----------|
| **Package imports** | `"github.com/nrfta/paging-go"` | `"github.com/nrfta/paging-go/v2/offset"` |
| **Constructor** | `paging.NewOffsetPaginator(pageArgs, totalCount)` | `offset.New(fetcher)` |
| **Page size config** | Constructor parameter | `Paginate()` options |
| **Pagination method** | Manual count + fetch + `QueryMods()` | `paginator.Paginate(ctx, page, opts...)` |
| **Result type** | Raw items + separate PageInfo | `*Page[T]` with Nodes, PageInfo, Metadata |
| **BuildConnection** | Manual edge/node array building | `offset.BuildConnection(result, transform)` |
| **Cursor strategy** | `paging.NewCursorPaginator(...)` | `cursor.New(fetcher, schema)` |
| **Quota-fill** | Not available | `quotafill.New(fetcher, filter, schema, opts...)` |

## Quick Summary

**What you need to change:**

1. Create a `Fetcher[T]` using `sqlboiler.NewFetcher()` with query and count functions
2. Pass fetcher to strategy constructor: `offset.New(fetcher)` instead of `offset.New(pageArgs, totalCount)`
3. Move page size config from constructor to `Paginate()` options: `paging.WithMaxSize(100)`
4. Replace manual fetch + `QueryMods()` with `paginator.Paginate(ctx, page, opts...)`
5. Update `BuildConnection()` calls to pass `*Page[T]` result instead of raw items

**What stays the same:**

- `paging.PageArgs` usage
- `paging.PageInfo` type and accessors
- `paging.Connection[T]` and `Edge[T]` types
- Transform functions signature
- SQLBoiler integration pattern

**What you gain:**

- ✨ **Consistent API** across all three pagination strategies
- ✨ **Reusable fetchers** - define once, use across multiple requests
- ✨ **Per-request page size limits** via functional options
- ✨ **Rich metadata** for observability (timing, strategy name, strategy-specific info)
- ✨ **Easier testing** with mockable `Fetcher[T]` interface
- ✨ **Simpler strategy switching** - change three lines of code to switch strategies

## Core Concept: The Fetcher Pattern

The biggest change in v2.0 is the introduction of the `Fetcher[T]` interface. Instead of passing `PageArgs` and `totalCount` to constructors, you now create a reusable fetcher that handles database queries.

**Benefits:**
- **Reusable**: Define once, use across multiple requests with different page sizes
- **ORM-agnostic**: Works with SQLBoiler, GORM, sqlc, or raw SQL
- **Strategy-agnostic**: Same fetcher works for offset, cursor, and quota-fill
- **Testable**: Easy to mock for unit tests

**The Fetcher interface:**

```go
type Fetcher[T any] interface {
    Fetch(ctx context.Context, params FetchParams) ([]T, error)
    Count(ctx context.Context, params FetchParams) (int64, error)
}
```

## Breaking Changes

### Constructor signatures changed

All strategy constructors now take `Fetcher[T]` as their first parameter:

- **Offset**: `offset.New(fetcher)`
- **Cursor**: `cursor.New(fetcher, schema)`
- **Quota-fill**: `quotafill.New(fetcher, filter, schema, opts...)`

### Page size limits moved to Paginate() options

Constructor no longer accepts default limit parameter. Use functional options instead:

```go
result, err := paginator.Paginate(ctx, page,
    paging.WithMaxSize(100),      // Cap at 100 items
    paging.WithDefaultSize(25),   // Default to 25 when First is nil
)
```

### Paginate() method returns *Page[T]

Instead of manually fetching data and building connections, call `Paginate()`:

```go
// Returns *Page[T] with Nodes, PageInfo, and Metadata populated
result, err := paginator.Paginate(ctx, page, opts...)
```

### BuildConnection() takes *Page[T] instead of raw items

Connection builders now work with the `*Page[T]` result:

```go
// Old: pass paginator + raw items
return offset.BuildConnection(paginator, dbUsers, transform)

// New: pass result from Paginate()
return offset.BuildConnection(result, transform)
```

## Migration Steps

### 1. Create a Fetcher

First, create a `Fetcher[T]` using `sqlboiler.NewFetcher()`:

```go
import (
    "github.com/nrfta/paging-go/v2/sqlboiler"
    "github.com/volatiletech/sqlboiler/v4/queries/qm"
)

fetcher := sqlboiler.NewFetcher(
    // Query function: returns slice of items
    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
        return models.Users(mods...).All(ctx, r.DB)
    },
    // Count function: returns total count
    func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
        return models.Users(mods...).Count(ctx, r.DB)
    },
    // Converter function: transforms FetchParams to query mods
    sqlboiler.OffsetToQueryMods,  // For offset pagination
    // OR: sqlboiler.CursorToQueryMods for cursor/quota-fill
)
```

**Key points:**
- Query function includes filters, joins, etc. but NOT limit/offset/order (handled by converter)
- Count function uses same filters as query function
- Choose converter based on pagination strategy: `OffsetToQueryMods` or `CursorToQueryMods`

### 2. Update Constructor Calls

Pass the fetcher to strategy constructors instead of PageArgs and count:

**Offset pagination:**

```go
// Old v0.3.0
totalCount, _ := models.Users().Count(ctx, r.DB)
paginator := offset.New(page, totalCount)

// New v2.0
fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.OffsetToQueryMods)
paginator := offset.New(fetcher)
```

**Cursor pagination:**

```go
// Old v0.3.0
fetchParams, _ := cursor.BuildFetchParams(page, schema)
users, _ := fetcher.Fetch(ctx, fetchParams)
paginator, _ := cursor.New(page, schema, users)

// New v2.0
fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.CursorToQueryMods)
paginator := cursor.New(fetcher, schema)
```

**Quota-fill pagination:**

```go
// Old v0.3.0
paginator := quotafill.New(fetcher, filter, schema, opts...)
result, _ := paginator.Paginate(ctx, page)

// New v2.0 (same, but fetcher creation is now explicit)
fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.CursorToQueryMods)
paginator := quotafill.New(fetcher, filter, schema, opts...)
result, _ := paginator.Paginate(ctx, page, paging.WithMaxSize(50))
```

### 3. Replace Manual Fetch with Paginate()

Instead of calling `QueryMods()` and manually fetching, call `Paginate()`:

**Offset example:**

```go
// Old v0.3.0
paginator := offset.New(page, totalCount)
dbUsers, err := models.Users(paginator.QueryMods()...).All(ctx, r.DB)
if err != nil {
    return nil, err
}
return offset.BuildConnection(paginator, dbUsers, toDomainUser)

// New v2.0
paginator := offset.New(fetcher)
result, err := paginator.Paginate(ctx, page,
    paging.WithMaxSize(100),
    paging.WithDefaultSize(25),
)
if err != nil {
    return nil, err
}
return offset.BuildConnection(result, toDomainUser)
```

**Cursor example:**

```go
// Old v0.3.0
fetchParams, _ := cursor.BuildFetchParams(page, schema)
users, _ := fetcher.Fetch(ctx, fetchParams)
paginator, _ := cursor.New(page, schema, users)
return cursor.BuildConnection(paginator, users, toDomainUser)

// New v2.0
paginator := cursor.New(fetcher, schema)
result, err := paginator.Paginate(ctx, page, paging.WithMaxSize(100))
if err != nil {
    return nil, err
}
return cursor.BuildConnection(result, schema, page, toDomainUser)
```

### 4. Update BuildConnection() Calls

All `BuildConnection()` functions now take `*Page[T]` as first parameter:

**Offset:**

```go
// Old: pass paginator + items
offset.BuildConnection(paginator, items, transform)

// New: pass result from Paginate()
offset.BuildConnection(result, transform)
```

**Cursor:**

```go
// Old: pass paginator + items
cursor.BuildConnection(paginator, items, transform)

// New: pass result + schema + page
cursor.BuildConnection(result, schema, page, transform)
```

**Quota-fill:**

```go
// Old: manual loop with edges
edges := make([]*paging.Edge[*Org], len(result.Nodes))
for i, org := range result.Nodes {
    domain, _ := toDomain(org)
    cursorStr, _ := schema.Encode(org)
    edges[i] = &paging.Edge[*Org]{Cursor: *cursorStr, Node: domain}
}
return &paging.Connection[*Org]{Edges: edges, Nodes: nodes, PageInfo: result.PageInfo}, nil

// New: use BuildConnection helper
quotafill.BuildConnection(result, schema, page, toDomain)
```

### 5. Move Page Size Config to Options

Page size limits now passed via functional options to `Paginate()`:

```go
// Old v0.3.0 (constructor parameter)
defaultLimit := 25
paginator := offset.New(page, totalCount, &defaultLimit)

// New v2.0 (Paginate options)
paginator := offset.New(fetcher)
result, err := paginator.Paginate(ctx, page,
    paging.WithMaxSize(100),      // Maximum allowed size
    paging.WithDefaultSize(25),   // Default when First is nil
)
```

**Benefits:**
- Configure page size per-request without creating new paginator
- Compose multiple options together
- Easier to add new options in the future

## Complete Example: Offset Pagination

### Before (v0.3.0)

```go
package resolvers

import (
    "context"
    "github.com/nrfta/paging-go/v2"
    "github.com/nrfta/paging-go/v2/offset"
    "github.com/my-user/my-app/models"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    // Get total count
    totalCount, err := models.Users().Count(ctx, r.DB)
    if err != nil {
        return nil, err
    }

    // Create paginator
    paginator := offset.New(page, totalCount)

    // Fetch records
    dbUsers, err := models.Users(paginator.QueryMods()...).All(ctx, r.DB)
    if err != nil {
        return nil, err
    }

    // Build connection
    return offset.BuildConnection(paginator, dbUsers, toDomainUser)
}

func toDomainUser(db *models.User) (*User, error) {
    return &User{ID: db.ID, Name: db.Name, Email: db.Email}, nil
}
```

### After (v2.0)

```go
package resolvers

import (
    "context"
    "github.com/nrfta/paging-go/v2"
    "github.com/nrfta/paging-go/v2/offset"
    "github.com/nrfta/paging-go/v2/sqlboiler"
    "github.com/my-user/my-app/models"
    "github.com/volatiletech/sqlboiler/v4/queries/qm"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    // 1. Create fetcher (once, reusable)
    fetcher := sqlboiler.NewFetcher(
        func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
            return models.Users(mods...).All(ctx, r.DB)
        },
        func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
            return models.Users(mods...).Count(ctx, r.DB)
        },
        sqlboiler.OffsetToQueryMods,
    )

    // 2. Create paginator (once, reusable)
    paginator := offset.New(fetcher)

    // 3. Paginate with per-request options
    result, err := paginator.Paginate(ctx, page,
        paging.WithMaxSize(100),
        paging.WithDefaultSize(25),
    )
    if err != nil {
        return nil, err
    }

    // 4. Build connection
    return offset.BuildConnection(result, toDomainUser)
}

func toDomainUser(db *models.User) (*User, error) {
    return &User{ID: db.ID, Name: db.Name, Email: db.Email}, nil
}
```

## Complete Example: Cursor Pagination

### Before (v0.3.0)

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    schema := cursor.NewSchema[*models.User]().
        Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
        FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })

    fetcher := sqlboiler.NewFetcher(
        func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
            mods = append([]qm.QueryMod{qm.Where("is_active = ?", true)}, mods...)
            return models.Users(mods...).All(ctx, r.DB)
        },
        func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
            return 0, nil
        },
        sqlboiler.CursorToQueryMods,
    )

    fetchParams, err := cursor.BuildFetchParams(page, schema)
    if err != nil {
        return nil, err
    }

    users, err := fetcher.Fetch(ctx, fetchParams)
    if err != nil {
        return nil, err
    }

    paginator, err := cursor.New(page, schema, users)
    if err != nil {
        return nil, err
    }

    return cursor.BuildConnection(paginator, users, toDomainUser)
}
```

### After (v2.0)

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    // 1. Define schema (once, reusable)
    schema := cursor.NewSchema[*models.User]().
        Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
        FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })

    // 2. Create fetcher (once, reusable)
    fetcher := sqlboiler.NewFetcher(
        func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
            mods = append([]qm.QueryMod{qm.Where("is_active = ?", true)}, mods...)
            return models.Users(mods...).All(ctx, r.DB)
        },
        func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
            return 0, nil
        },
        sqlboiler.CursorToQueryMods,
    )

    // 3. Create paginator (once, reusable)
    paginator := cursor.New(fetcher, schema)

    // 4. Paginate
    result, err := paginator.Paginate(ctx, page, paging.WithMaxSize(100))
    if err != nil {
        return nil, err
    }

    // 5. Build connection
    return cursor.BuildConnection(result, schema, page, toDomainUser)
}
```

## What Stays the Same

These parts of the API remain unchanged:

- **`paging.PageArgs`**: Structure and usage identical
- **`paging.WithSortBy()`**: Helper function unchanged
- **`QueryMods()`** method: Usage identical
- **SQLBoiler integration**: Works the same way

## Cursor Functions

Cursor encoding/decoding functions have moved to the offset package and been renamed:

| Old (v0.3.0) | New (v2.0) |
|--------------|------------|
| `paging.EncodeOffsetCursor()` | `offset.EncodeCursor()` |
| `paging.DecodeOffsetCursor()` | `offset.DecodeCursor()` |

**Before:**

```go
cursor := paging.EncodeOffsetCursor(20)
offsetValue := paging.DecodeOffsetCursor(cursor)
```

**After:**

```go
cursor := offset.EncodeCursor(20)
offsetValue := offset.DecodeCursor(cursor)
```

**Note:** Most users don't need to call these functions directly - the paginator and `BuildConnection()` handle cursor encoding/decoding automatically. You only need these if you're manually building cursors for testing or custom pagination logic.

## New: Automatic N+1 Pattern for Cursor Pagination

v2.0's unified `Paginate()` method automatically handles the N+1 pattern for all strategies, eliminating the need to manually add +1 to your limit.

**Before (Manual N+1):**

```go
// ❌ Had to remember to add +1 manually
limit := 10
if page != nil && page.First != nil {
    limit = *page.First
}

var cursorPos *paging.CursorPosition
if page != nil && page.After != nil {
    cursorPos, _ = encoder.Decode(*page.After)
}

fetchParams := paging.FetchParams{
    Limit:   limit + 1,  // Manual N+1
    Cursor:  cursorPos,
    OrderBy: []paging.Sort{
        {Column: "created_at", Desc: true},
        {Column: "id", Desc: true},
    },
}
users, _ := fetcher.Fetch(ctx, fetchParams)
```

**After (Automatic N+1):**

```go
// ✅ N+1 handled automatically!
sorts := []paging.Sort{
    {Column: "created_at", Desc: true},
    {Column: "id", Desc: true},
}
fetchParams := cursor.BuildFetchParams(page, encoder, sorts)
users, _ := fetcher.Fetch(ctx, fetchParams)
```

This makes cursor pagination consistent with offset and quota-fill pagination, where N+1 is handled internally. The function:

- Extracts the requested limit from PageArgs (defaults to 50)
- Automatically adds +1 for HasNextPage detection
- Decodes the After cursor
- Returns ready-to-use FetchParams

## Reusability Benefits

The new API encourages reusability at every level:

**Fetcher reusability:**

```go
// Define once (e.g., in a repository struct)
type UserRepository struct {
    db      *sql.DB
    fetcher paging.Fetcher[*models.User]
}

func NewUserRepository(db *sql.DB) *UserRepository {
    return &UserRepository{
        db: db,
        fetcher: sqlboiler.NewFetcher(
            func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
                return models.Users(mods...).All(ctx, db)
            },
            func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
                return models.Users(mods...).Count(ctx, db)
            },
            sqlboiler.OffsetToQueryMods,
        ),
    }
}

// Use across multiple methods
func (r *UserRepository) ListWithOffset(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    paginator := offset.New(r.fetcher)
    result, _ := paginator.Paginate(ctx, page, paging.WithMaxSize(100))
    return offset.BuildConnection(result, toDomain)
}

func (r *UserRepository) ListWithCursor(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
    // Same fetcher, different strategy!
    paginator := cursor.New(r.fetcher, userSchema)
    result, _ := paginator.Paginate(ctx, page, paging.WithMaxSize(100))
    return cursor.BuildConnection(result, userSchema, page, toDomain)
}
```

**Schema reusability (cursor/quota-fill):**

```go
// Define once at package level
var userSchema = cursor.NewSchema[*models.User]().
    Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
    Field("name", "n", func(u *models.User) any { return u.Name }).
    FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })

// Reuse in both cursor and quota-fill paginators
```

**Per-request configuration:**

```go
// Same paginator, different page sizes per endpoint
adminPaginator := offset.New(fetcher)

// Admin endpoint: large pages
result, _ := adminPaginator.Paginate(ctx, page, paging.WithMaxSize(500))

// Public API endpoint: smaller pages
result, _ := adminPaginator.Paginate(ctx, page, paging.WithMaxSize(50))
```

## Why This Change?

The unified API with Fetcher pattern provides significant benefits:

1. **Consistent API**: All three strategies work the same way - learn once, use everywhere
2. **Better reusability**: Define fetchers, paginators, and schemas once, use across multiple requests
3. **Per-request configuration**: No need to create new paginators for different page sizes
4. **Easier testing**: Mock the `Fetcher[T]` interface for unit tests
5. **Simpler strategy switching**: Change three lines of code to switch from offset to cursor
6. **Rich metadata**: Observability for monitoring, debugging, and performance optimization
7. **ORM-agnostic**: Same pattern works with SQLBoiler, GORM, sqlc, or raw SQL
8. **Less boilerplate**: Single `Paginate()` call replaces manual fetch + trim logic

## Migration Checklist

### For Offset Pagination:
- [ ] Add `sqlboiler` package import
- [ ] Create `Fetcher[T]` using `sqlboiler.NewFetcher()`
- [ ] Update constructor: `offset.New(page, totalCount)` → `offset.New(fetcher)`
- [ ] Replace manual fetch with `paginator.Paginate(ctx, page, opts...)`
- [ ] Move page size config from constructor to `WithMaxSize()` / `WithDefaultSize()` options
- [ ] Update `BuildConnection()`: pass `result` instead of `paginator` + `items`
- [ ] Run tests to verify everything works

### For Cursor Pagination:
- [ ] Create `Fetcher[T]` using `sqlboiler.NewFetcher()` with `CursorToQueryMods`
- [ ] Update constructor: `cursor.New(page, schema, items)` → `cursor.New(fetcher, schema)`
- [ ] Remove manual `BuildFetchParams()` and `fetcher.Fetch()` calls
- [ ] Replace with `paginator.Paginate(ctx, page, opts...)`
- [ ] Update `BuildConnection()`: pass `result, schema, page, transform` instead of `paginator, items, transform`
- [ ] Run tests to verify everything works

### For Quota-fill Pagination:
- [ ] Create `Fetcher[T]` using `sqlboiler.NewFetcher()` with `CursorToQueryMods`
- [ ] Constructor already takes fetcher (no change needed)
- [ ] Add per-request options to `Paginate()` call
- [ ] Replace manual connection building loop with `quotafill.BuildConnection()`
- [ ] Run tests to verify everything works

### General:
- [ ] Review metadata access patterns if needed
- [ ] Consider refactoring to reuse fetchers, paginators, and schemas
- [ ] Update documentation and comments to reflect new API
- [ ] Enjoy the cleaner, more consistent API! 🎉

## Need Help?

If you encounter issues during migration, please:

1. Check this guide thoroughly
2. Review the [README.md](./README.md) for examples
3. Check the test files for usage patterns
4. Open an issue on GitHub with your specific use case
