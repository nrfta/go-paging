# go-paging ![](https://github.com/nrfta/go-paging/workflows/CI/badge.svg)

Modern Go pagination for [SQLBoiler](https://github.com/aarondl/sqlboiler) and [gqlgen](https://github.com/99designs/gqlgen/) (GraphQL).

**New in v2:** Built-in Connection/Edge builders eliminate 60-80% of boilerplate code.

## Features

- **Relay-compliant GraphQL pagination** with automatic Connection/Edge building
- **Type-safe transformations** from database models to domain models
- **Modular architecture** with pluggable pagination strategies (offset + cursor)
- **High-performance cursor pagination** for large datasets (O(1) performance)
- **SQLBoiler integration** with query mod support

## Install

```sh
go get -u "github.com/nrfta/go-paging"
```

## Quick Start

### 1. Add GraphQL Schema

Add [this GraphQL schema](./schema.graphql) to your project, and configure gqlgen:

```yaml
# gqlgen.yml
models:
  PageArgs:
    model: github.com/nrfta/go-paging.PageArgs
  PageInfo:
    model: github.com/nrfta/go-paging.PageInfo
```

### 2. Add PageInfo Resolver

```go
package resolvers

import "github.com/nrfta/go-paging"

func (r *RootResolver) PageInfo() PageInfoResolver {
 return paging.NewPageInfoResolver()
}
```

### 3. Use in Resolvers

**GraphQL Schema:**

```graphql
type User {
  id: ID!
  name: String!
  email: String!
}

type UserEdge {
  cursor: String!
  node: User!
}

type UserConnection {
  edges: [UserEdge!]!
  nodes: [User!]!
  pageInfo: PageInfo!
}

type Query {
  users(page: PageArgs): UserConnection!
}
```

**Resolver (New API - Recommended):**

```go
package resolvers

import (
 "context"

 "github.com/nrfta/go-paging"
 "github.com/nrfta/go-paging/offset"

 "github.com/my-user/my-app/models"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[User], error) {
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

 // Build connection with automatic edge/node creation - ONE LINE!
 return offset.BuildConnection(paginator, dbUsers, toDomainUser)
}

// Transform function (database model → domain model)
func toDomainUser(db *models.User) (*User, error) {
 return &User{
  ID:    db.ID,
  Name:  db.Name,
  Email: db.Email,
 }, nil
}
```

**That's it!** No manual loops, no cursor encoding, no edge building boilerplate.

## Migration from v1

See [MIGRATION.md](./MIGRATION.md) for details.

**Quick changes:**

1. Import: `"github.com/nrfta/go-paging/offset"`
2. Change: `paging.NewOffsetPaginator()` → `offset.New()`
3. Use: `offset.BuildConnection()` to eliminate boilerplate

## Advanced Usage

### Custom Page Size

```go
// Default limit is 50
paginator := offset.New(pageArgs, totalCount)

// Custom default limit
defaultLimit := 25
paginator := offset.New(pageArgs, totalCount, &defaultLimit)
```

### Custom Sorting

```go
// Sort by created_at descending
pageArgs := paging.WithSortBy(nil, true, "created_at")

// Sort by multiple columns
pageArgs := paging.WithSortBy(nil, true, "created_at", "id")
```

### Direct Query Mods (SQLBoiler)

If you prefer to work with SQLBoiler query mods directly:

```go
paginator := offset.New(pageArgs, totalCount)
mods := paginator.QueryMods()

// Add custom filters
mods = append(mods, qm.Where("status = ?", "active"))

records, err := models.Users(mods...).All(ctx, db)
```

## Cursor-Based Pagination

For high-performance pagination of large datasets (millions of records), use cursor-based (keyset) pagination instead of offset.

**Key Benefits:**
- **O(1) performance** - Page 1 and page 1,000,000 have the same query time
- **Consistent results** - No duplicate/missing records when data changes during pagination
- **Index-friendly** - Uses composite indexes for fast lookups

### Basic Usage

```go
import (
    "github.com/nrfta/go-paging/cursor"
    "github.com/nrfta/go-paging/sqlboiler"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[User], error) {
    // Create encoder (defines cursor structure)
    encoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
        return map[string]any{
            "created_at": u.CreatedAt,
            "id":         u.ID,
        }
    })

    // Create fetcher with cursor strategy
    fetcher := sqlboiler.NewFetcher(
        func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
            // ✅ Add filters only - NO qm.OrderBy here!
            mods = append([]qm.QueryMod{
                qm.Where("is_active = ?", true),
            }, mods...)
            return models.Users(mods...).All(ctx, r.DB)
        },
        func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
            return 0, nil // Count not used for cursor pagination
        },
        sqlboiler.CursorToQueryMods, // Use cursor strategy
    )

    // Decode cursor from PageArgs
    var cursorPos *paging.CursorPosition
    if page != nil && page.After != nil {
        cursorPos, _ = encoder.Decode(*page.After)
    }

    // Fetch with pagination (N+1 pattern for accurate HasNextPage)
    limit := 10
    if page != nil && page.First != nil {
        limit = *page.First
    }

    fetchParams := paging.FetchParams{
        Limit:   limit + 1, // Fetch one extra to detect if there's a next page
        Cursor:  cursorPos,
        OrderBy: []paging.OrderBy{
            {Column: "created_at", Desc: true},
            {Column: "id", Desc: true},
        },
    }
    users, err := fetcher.Fetch(ctx, fetchParams)
    if err != nil {
        return nil, err
    }

    // Build connection (paginator automatically trims to limit if we got limit+1)
    paginator := cursor.New(page, encoder, users)
    return cursor.BuildConnection(paginator, users, encoder, toDomainUser)
}
```

### N+1 Pattern for Accurate HasNextPage

Cursor pagination uses the **N+1 pattern** to accurately determine if there's a next page:

1. **Fetch LIMIT + 1** records from the database
2. If you get LIMIT + 1 records, `HasNextPage = true`
3. The paginator automatically trims to LIMIT records for the response

```go
limit := 10
fetchParams := paging.FetchParams{
    Limit: limit + 1,  // Fetch one extra for HasNextPage detection
    // ...
}
users, _ := fetcher.Fetch(ctx, fetchParams)

paginator := cursor.New(page, encoder, users)  // Detects N+1, trims to 10
conn, _ := cursor.BuildConnection(paginator, users, encoder, transform)
// conn.Nodes has 10 items, conn.PageInfo.HasNextPage() is accurate
```

### ⚠️ Critical: ORDER BY Rules

**DO NOT add `qm.OrderBy()` to your query mods when using cursor pagination.** This will cause duplicate records and data corruption.

**❌ WRONG - Causes duplicates:**
```go
fetcher := sqlboiler.NewFetcher(
    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
        // ❌ DO NOT DO THIS - conflicts with cursor's ORDER BY
        mods = append([]qm.QueryMod{
            qm.OrderBy("name ASC"),
        }, mods...)
        return models.Users(mods...).All(ctx, r.DB)
    },
    countFunc,
    sqlboiler.CursorToQueryMods,
)
```

**Why?** Cursor pagination generates a WHERE clause like:
```sql
WHERE (created_at < ? OR (created_at = ? AND id < ?))
```

This WHERE clause **assumes** `created_at` and `id` are the primary sort columns. If you add `ORDER BY name ASC`, the actual query becomes:
```sql
ORDER BY name ASC, created_at DESC, id DESC
```

Now `name` is the primary sort, but the WHERE clause still filters by `created_at`/`id` → **wrong results and duplicates!**

**✅ CORRECT - Define sorting in FetchParams:**
```go
fetcher := sqlboiler.NewFetcher(
    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
        // ✅ Filters only - sorting handled by FetchParams
        mods = append([]qm.QueryMod{
            qm.Where("is_active = ?", true),
        }, mods...)
        return models.Users(mods...).All(ctx, r.DB)
    },
    countFunc,
    sqlboiler.CursorToQueryMods,
)

// ✅ Define sorting here
fetchParams := paging.FetchParams{
    OrderBy: []paging.OrderBy{
        {Column: "name", Desc: false},
        {Column: "id", Desc: false},  // Always include unique column last
    },
}
```

### Required Database Index

For optimal performance, create a composite index matching your sort columns:

```sql
-- For sorting by (created_at DESC, id DESC)
CREATE INDEX idx_users_cursor ON users(created_at DESC, id DESC);

-- For sorting by (name ASC, id ASC)
CREATE INDEX idx_users_name_cursor ON users(name ASC, id ASC);
```

### Cursor vs Offset: When to Use Which

| Feature | Offset | Cursor |
|---------|--------|--------|
| **Performance on page 1** | Fast (5ms) | Fast (5ms) |
| **Performance on page 1000** | Slow (1000ms+) | Fast (5ms) |
| **Jump to page N** | ✅ Yes | ❌ No (forward-only) |
| **Total count** | ✅ Yes | ❌ No |
| **Consistent during writes** | ❌ Can skip/duplicate | ✅ Consistent |
| **Best for** | Admin UIs, reports | Feeds, infinite scroll |

**Use offset when:**
- You need page numbers (1, 2, 3...)
- You need total count
- Dataset is small (< 10,000 records)
- Users jump to specific pages

**Use cursor when:**
- Dataset is large (> 100,000 records)
- Infinite scroll / "Load More" UI
- Data changes frequently during pagination
- Performance is critical

## Architecture

```
go-paging/
├── connection.go          # Generic Connection[T] and Edge[T] types
├── interfaces.go          # Core pagination interfaces (Paginator[T], Fetcher[T])
├── models.go              # PageArgs and PageInfo
├── offset/                # Offset-based pagination
│   ├── paginator.go       # Offset paginator + BuildConnection
│   └── cursor.go          # Offset cursor encoding
├── cursor/                # Cursor-based (keyset) pagination
│   ├── paginator.go       # Cursor paginator + BuildConnection
│   └── encoder.go         # Composite cursor encoding/decoding
└── sqlboiler/             # SQLBoiler ORM adapter
    ├── fetcher.go         # Generic Fetcher[T] (ORM integration)
    ├── offset.go          # Offset query builder (strategy-specific)
    └── cursor.go          # Cursor query builder (strategy-specific)
```

**Design Philosophy:**

- **ORM adapters** (sqlboiler, gorm, etc.) are generic and strategy-agnostic
- **Query builders** (offset, cursor) are strategy-specific
- **Easy to extend:** Add new strategies without changing ORM adapters

## Roadmap

**Phase 1 (v2.0):** ✅

- Connection/Edge builders
- Modular architecture
- Offset pagination

**Phase 2 (Current):** ✅

- Cursor-based pagination (keyset pagination)
- High-performance for large datasets
- O(1) complexity regardless of page depth
- Forward pagination (After + First)

**Phase 3 (Planned):**

- Backward pagination (Before + Last) for cursor strategy
- Quota-fill pagination wrapper
- Authorization-aware filtering
- Consistent page sizes with per-item filtering

## Contributing

Contributions welcome! Please open an issue or PR.

## License

This project is licensed under the [MIT License](LICENSE.md).
