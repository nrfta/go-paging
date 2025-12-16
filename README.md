# paging-go ![](https://github.com/nrfta/paging-go/workflows/CI/badge.svg)

Type-safe Relay pagination for [SQLBoiler](https://github.com/aarondl/sqlboiler) and [gqlgen](https://github.com/99designs/gqlgen/).

Supports three pagination strategies: offset (traditional LIMIT/OFFSET), cursor (keyset pagination), and quota-fill (filter-aware iterative fetching). All strategies provide automatic Connection/Edge building and eliminate boilerplate through generic builders.

## Install

```sh
go get -u "github.com/nrfta/paging-go/v2"
```

## Migration from v0.3.0

Breaking changes in v2.0 moved from monolithic API to modular packages and unified all strategies around a common `Paginator[T]` interface. See [MIGRATION.md](./MIGRATION.md) for details.

Quick summary:
1. Import strategy packages: `"github.com/nrfta/paging-go/v2/offset"`
2. Create a `Fetcher[T]` using `sqlboiler.NewFetcher()`
3. Change constructors: `paging.NewOffsetPaginator()` → `offset.New(fetcher)`
4. Call `Paginate()` method instead of manual fetching
5. Use `BuildConnection()` helpers to eliminate boilerplate

The new API is consistent across all three strategies, making it easier to switch between them or use multiple strategies in the same codebase.

## API Design Philosophy

All three pagination strategies follow the same unified pattern, making them easy to learn and switch between:

**1. Fetcher Pattern:** Create a reusable `Fetcher[T]` that handles database queries. The fetcher is ORM-agnostic and strategy-agnostic - it works with SQLBoiler, GORM, sqlc, or raw SQL. Define it once, use it across multiple requests.

**2. Paginator Interface:** Each strategy implements `Paginator[T]` with the same `Paginate()` method signature. The only difference is constructor parameters (offset needs nothing, cursor needs schema, quota-fill needs filter and schema).

**3. Functional Options:** Configure page size limits per-request using options like `WithMaxSize(100)` and `WithDefaultSize(25)`. No need to create new paginators for different limits.

**4. BuildConnection Helpers:** Eliminate 60-80% of boilerplate by using strategy-specific `BuildConnection()` functions that handle edge creation, cursor generation, and model transformation.

This design means you can switch from offset to cursor pagination by changing three lines of code, not rewriting your entire resolver.

## Quick Start

### GraphQL Setup

Add [this schema](./schema.graphql) and configure gqlgen:

```yaml
# gqlgen.yml
models:
  PageArgs:
    model: github.com/nrfta/paging-go/v2.PageArgs
  PageInfo:
    model: github.com/nrfta/paging-go/v2.PageInfo
```

### PageInfo Resolver

```go
package resolvers

import "github.com/nrfta/paging-go/v2"

func (r *RootResolver) PageInfo() PageInfoResolver {
  return paging.NewPageInfoResolver()
}
```

### Basic Resolver Example

Using offset pagination (simplest entry point):

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
  // 1. Create fetcher (once, reusable across requests)
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
    paging.WithMaxSize(100),      // Cap at 100 items
    paging.WithDefaultSize(25),   // Default to 25 when First is nil
  )
  if err != nil {
    return nil, err
  }

  // 4. Build connection with automatic edge/node creation
  return offset.BuildConnection(result, toDomainUser)
}

// Transform database model to domain model
func toDomainUser(db *models.User) (*User, error) {
  return &User{ID: db.ID, Name: db.Name, Email: db.Email}, nil
}
```

## Pagination Strategies

### Offset Pagination

Traditional LIMIT/OFFSET with page numbers. Best for small-to-medium datasets where users need page navigation.

**Use cases:**
- Admin UIs with page numbers
- Reports and exports
- Datasets under 10,000 records
- Total count required

**Performance:**
- Page 1: Fast (5ms)
- Page 1000: Slow (1000ms+) - database scans all preceding rows

**Custom configuration:**

```go
// Page size limits are passed via options to Paginate()
result, err := paginator.Paginate(ctx, page,
  paging.WithMaxSize(200),      // Cap at 200 items
  paging.WithDefaultSize(50),   // Default to 50 when First is nil
)

// Single column sort (use PageArgs helpers)
pageArgs := paging.WithSortBy(nil, "created_at", true)

// Multi-column sort
pageArgs := paging.WithMultiSort(nil,
  paging.Sort{Column: "created_at", Desc: true},
  paging.Sort{Column: "name", Desc: false},
  paging.Sort{Column: "id", Desc: true},
)
```

### Cursor Pagination

High-performance keyset pagination using composite indexes. Provides O(1) performance regardless of page depth.

**Use cases:**
- Large datasets (100,000+ records)
- Infinite scroll / "Load More" UIs
- Real-time feeds
- Performance-critical applications

**Performance:**
- All pages: Fast (5ms) - constant time regardless of depth

**Implementation:**

```go
import (
  "github.com/nrfta/paging-go/v2"
  "github.com/nrfta/paging-go/v2/cursor"
  "github.com/nrfta/paging-go/v2/sqlboiler"
  "github.com/volatiletech/sqlboiler/v4/queries/qm"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*User], error) {
  // 1. Define schema (cursor fields and ordering)
  schema := cursor.NewSchema[*models.User]().
    Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
    FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })

  // 2. Create fetcher (once, reusable)
  fetcher := sqlboiler.NewFetcher(
    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
      // Add filters only - NO qm.OrderBy here
      mods = append([]qm.QueryMod{
        qm.Where("is_active = ?", true),
      }, mods...)
      return models.Users(mods...).All(ctx, r.DB)
    },
    func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
      return 0, nil // Count not used for cursor pagination
    },
    sqlboiler.CursorToQueryMods,
  )

  // 3. Create paginator (once, reusable)
  paginator := cursor.New(fetcher, schema)

  // 4. Paginate with per-request options
  result, err := paginator.Paginate(ctx, page, paging.WithMaxSize(100))
  if err != nil {
    return nil, err
  }

  // 5. Build connection with transformation
  return cursor.BuildConnection(result, schema, page, toDomainUser)
}
```

**Critical: ORDER BY rules**

ORDER BY clauses must be defined in the schema, not in query mods. Adding `qm.OrderBy()` to the fetcher causes duplicate records and incorrect results.

**Why:** Cursor pagination generates WHERE clauses based on sort columns from the schema. If you add `qm.OrderBy()` in the fetcher function, you'll have conflicting ORDER BY clauses that produce wrong results.

```go
// WRONG - Causes duplicates
fetcher := sqlboiler.NewFetcher(
  func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
    mods = append([]qm.QueryMod{
      qm.OrderBy("name ASC"), // Conflicts with cursor's ORDER BY
    }, mods...)
    return models.Users(mods...).All(ctx, r.DB)
  },
  countFunc,
  sqlboiler.CursorToQueryMods,
)

// CORRECT - Define sorting in schema
fetcher := sqlboiler.NewFetcher(
  func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
    mods = append([]qm.QueryMod{
      qm.Where("is_active = ?", true), // Filters only
    }, mods...)
    return models.Users(mods...).All(ctx, r.DB)
  },
  countFunc,
  sqlboiler.CursorToQueryMods,
)

// Define sorting in schema
schema := cursor.NewSchema[*models.User]().
  Field("name", "n", func(u *models.User) any { return u.Name }).
  FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })
```

**Required database index:**

Create a composite index matching sort columns:

```sql
-- For sorting by (created_at DESC, id DESC)
CREATE INDEX idx_users_cursor ON users(created_at DESC, id DESC);

-- For sorting by (name ASC, id ASC)
CREATE INDEX idx_users_name_cursor ON users(name ASC, id ASC);
```

### Schema Pattern

Schema provides a single source of truth for cursor encoding, sort ordering, security, and fixed fields.

```go
var userSchema = cursor.NewSchema[*models.User]().
  // Fixed field: Always sort by tenant_id first (for partitioning)
  FixedField("tenant_id", cursor.ASC, "t", func(u *models.User) any {
    return u.TenantID
  }).
  // User-sortable fields with short cursor keys
  Field("name", "n", func(u *models.User) any { return u.Name }).
  Field("email", "e", func(u *models.User) any { return u.Email }).
  Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
  // Fixed field: Always append ID for uniqueness
  FixedField("id", cursor.DESC, "i", func(u *models.User) any {
    return u.ID
  })
```

**Benefits:**
- **Security:** Cursors use short keys (`{"n": "Alice", "i": "123"}`) instead of column names
- **Type safety:** Schema validates sort fields before encoding
- **Automatic fixed fields:** Ensures required fields are always included in ORDER BY
- **Dynamic sorting:** Users can choose sort fields at runtime
- **JOIN support:** Use qualified column names without exposing them in cursors

**Multi-tenant with fixed prefix:**

```go
var userSchema = cursor.NewSchema[*models.User]().
  FixedField("tenant_id", cursor.ASC, "t", func(u *models.User) any {
    return u.TenantID
  }).
  Field("name", "n", func(u *models.User) any { return u.Name }).
  FixedField("id", cursor.DESC, "i", func(u *models.User) any {
    return u.ID
  })

// ORDER BY: tenant_id ASC, name DESC, id DESC
// Cursor: {"t": 42, "n": "Alice", "i": "123"}
// Efficient with (tenant_id, name, id) composite index
```

**JOIN query example:**

```go
type UserWithPost struct {
  UserID        string
  UserName      string
  PostID        string
  PostCreatedAt time.Time
}

var joinSchema = cursor.NewSchema[*UserWithPost]().
  Field("posts.created_at", "pc", func(uwp *UserWithPost) any {
    return uwp.PostCreatedAt
  }).
  Field("users.name", "un", func(uwp *UserWithPost) any {
    return uwp.UserName
  }).
  FixedField("posts.id", cursor.DESC, "pi", func(uwp *UserWithPost) any {
    return uwp.PostID
  })

// ORDER BY: posts.created_at DESC, users.name ASC, posts.id DESC
// Cursor: {"pc": "2024-01-01", "un": "Alice", "pi": "post-123"}
// No column name ambiguity, no schema leakage
```

**N+1 pattern:**

All strategies use N+1 pattern to detect next page availability:
1. Fetch LIMIT + 1 records
2. If result has LIMIT + 1 records, `HasNextPage = true`
3. Paginator trims to LIMIT for response

This is handled automatically - no manual +1 required.

### Quota-Fill Pagination

Iteratively fetches batches until requested page size is filled. Solves inconsistent page sizes when applying authorization filters or per-item filtering.

**Problem:** Standard pagination with filtering creates inconsistent page sizes:

```go
// Request 10 items, fetch 10 from DB
users, _ := fetcher.Fetch(ctx, limit: 10)

// Apply authorization filter - returns 3 items
authorized := filterAuthorized(users)

// User asked for 10, got 3 - inconsistent page size
return authorized
```

This creates poor UX: uneven layouts, unpredictable "Load More" behavior, multiple clicks to see full page.

**Solution:** Quota-fill iteratively fetches until quota is met:

```go
import (
  "time"
  "github.com/nrfta/paging-go/v2"
  "github.com/nrfta/paging-go/v2/cursor"
  "github.com/nrfta/paging-go/v2/quotafill"
  "github.com/nrfta/paging-go/v2/sqlboiler"
  "github.com/volatiletech/sqlboiler/v4/queries/qm"
)

func (r *queryResolver) Organizations(ctx context.Context, page *paging.PageArgs) (*paging.Connection[*Organization], error) {
  // 1. Define schema (reusable)
  schema := cursor.NewSchema[*models.Organization]().
    Field("created_at", "c", func(o *models.Organization) any { return o.CreatedAt }).
    FixedField("id", cursor.DESC, "i", func(o *models.Organization) any { return o.ID })

  // 2. Create fetcher (once, reusable)
  fetcher := sqlboiler.NewFetcher(
    func(ctx context.Context, mods ...qm.QueryMod) ([]*models.Organization, error) {
      mods = append([]qm.QueryMod{
        qm.Where("deleted_at IS NULL"), // Pre-filter in DB
      }, mods...)
      return models.Organizations(mods...).All(ctx, r.DB)
    },
    func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
      return 0, nil
    },
    sqlboiler.CursorToQueryMods,
  )

  // 3. Define authorization filter (applied after DB fetch)
  authFilter := func(ctx context.Context, orgs []*models.Organization) ([]*models.Organization, error) {
    return r.AuthzClient.FilterAuthorized(ctx, r.CurrentUser(ctx), orgs)
  }

  // 4. Create quota-fill paginator with strategy-specific options
  paginator := quotafill.New(fetcher, authFilter, schema,
    quotafill.WithMaxIterations(5),
    quotafill.WithMaxRecordsExamined(100),
    quotafill.WithTimeout(5 * time.Second),
  )

  // 5. Paginate with per-request options
  result, err := paginator.Paginate(ctx, page, paging.WithMaxSize(50))
  if err != nil {
    return nil, err
  }

  // 6. Check metadata for safeguard hits
  if result.Metadata.SafeguardHit != nil {
    log.Warnf("Safeguard hit: %s", *result.Metadata.SafeguardHit)
  }

  // 7. Build connection (quotafill has BuildConnection helper now)
  return quotafill.BuildConnection(result, schema, page, toDomainOrganization)
}
```

**Algorithm:**

1. Initialize: `filteredItems = []`, `iteration = 0`
2. Loop while `len(filteredItems) < requestedSize + 1`:
   - Calculate `fetchSize = (remaining quota) × backoffMultiplier[iteration]`
   - Fetch batch using fetcher
   - Apply filter function
   - Append filtered items to results
   - Check safeguards (maxIterations, maxRecordsExamined, timeout)
   - Break if no more data or safeguard triggered
3. Trim to `requestedSize` if N+1 items fetched
4. Return results with metadata

**Adaptive backoff:**

Uses Fibonacci-like multipliers `[1, 2, 3, 5, 8]` to optimize fetching:
- Iteration 1: Fetch exactly what's needed (1×)
- Iteration 2: Filter pass rate < 100%, overscan (2×)
- Iteration 3+: Progressively larger overscan (3×, 5×, 8×)

**Safeguards:**

Prevent infinite loops and excessive load:

```go
quotafill.New(fetcher, filter, schema,
  quotafill.WithMaxIterations(5),          // Default: 5
  quotafill.WithMaxRecordsExamined(100),   // Default: 100
  quotafill.WithTimeout(5 * time.Second),  // Default: 3s
)
```

When triggered, partial results are returned with metadata indicating which safeguard was hit.

**Metadata tracking:**

All strategies return metadata in `result.Metadata` for observability:

```go
result, err := paginator.Paginate(ctx, page)

fmt.Printf("Strategy: %s\n", result.Metadata.Strategy)               // "quotafill"
fmt.Printf("Query Time: %dms\n", result.Metadata.QueryTimeMs)        // 42

// Quota-fill specific metadata
if result.Metadata.ItemsExamined != nil {
  fmt.Printf("Items Examined: %d\n", *result.Metadata.ItemsExamined)
}
if result.Metadata.IterationsUsed != nil {
  fmt.Printf("Iterations Used: %d\n", *result.Metadata.IterationsUsed)
}
if result.Metadata.SafeguardHit != nil {
  fmt.Printf("Safeguard Hit: %s\n", *result.Metadata.SafeguardHit) // "max_iterations"
}

// Offset-specific metadata
if result.Metadata.Offset != nil {
  fmt.Printf("Current Offset: %d\n", *result.Metadata.Offset)
}
```

**Performance tips:**

1. Push filtering into database queries when possible:

```go
// Better: Pre-filter in database, quota-fill for edge cases
fetcher := sqlboiler.NewFetcher(
  func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
    mods = append([]qm.QueryMod{
      qm.Where("is_active = ?", true),           // Database filter
      qm.Where("department = ?", "engineering"), // Database filter
    }, mods...)
    return models.Users(mods...).All(ctx, db)
  },
  countFunc,
  sqlboiler.CursorToQueryMods,
)

// Quota-fill only for authorization checks
authFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
  return authzClient.FilterAuthorized(ctx, currentUser, users)
}
```

2. Monitor filter pass rates:
   - 90% pass rate: Usually 1 iteration
   - 50% pass rate: Usually 2 iterations
   - 10% pass rate: May hit safeguards

3. Set up alerts when safeguards trigger frequently.

## Architecture

```
go-paging/
├── connection.go          # Generic Connection[T], Edge[T], and Page[T] types
├── interfaces.go          # Core interfaces (Paginator[T], Fetcher[T], FilterFunc[T])
├── models.go              # PageArgs, PageInfo, Metadata, PaginateOption
├── offset/                # Offset-based pagination
│   ├── paginator.go       # Offset Paginator[T] + BuildConnection
│   └── cursor.go          # Offset cursor encoding
├── cursor/                # Cursor-based (keyset) pagination
│   ├── paginator.go       # Cursor Paginator[T] + BuildConnection
│   ├── encoder.go         # Composite cursor encoding/decoding
│   └── schema.go          # Schema definition for cursor fields
├── quotafill/             # Quota-fill pagination (iterative fetching)
│   ├── paginator.go       # Quota-fill Paginator[T] + BuildConnection
│   └── strategy.go        # Adaptive backoff and safeguards
└── sqlboiler/             # SQLBoiler ORM adapter
    ├── fetcher.go         # Generic Fetcher[T] implementation
    ├── offset.go          # OffsetToQueryMods converter
    └── cursor.go          # CursorToQueryMods converter
```

The architecture reflects the unified API design:

**Core Abstractions:**
- `Paginator[T]` interface: All strategies implement the same `Paginate(ctx, PageArgs, ...PaginateOption)` method
- `Fetcher[T]` interface: ORM-agnostic data fetching with query mods
- `Page[T]`: Result container with Nodes, PageInfo, and Metadata

**Strategy Packages:**
- Each strategy (offset/, cursor/, quotafill/) is an independent package
- All implement `Paginator[T]` with consistent API
- Each provides `BuildConnection()` helper to eliminate boilerplate
- Strategy-specific options passed to constructors, page size options passed to `Paginate()`

**ORM Adapters:**
- sqlboiler/ package provides `Fetcher[T]` implementation
- Converter functions (`OffsetToQueryMods`, `CursorToQueryMods`) transform `FetchParams` to ORM query mods
- Same fetcher pattern can be implemented for GORM, sqlc, or raw SQL

## Comparison: Offset vs Cursor

| Feature | Offset | Cursor |
|---------|--------|--------|
| Performance on page 1 | Fast (5ms) | Fast (5ms) |
| Performance on page 1000 | Slow (1000ms+) | Fast (5ms) |
| Jump to page N | Yes | No (forward-only) |
| Total count | Yes | No |
| Consistent during writes | Can skip/duplicate | Consistent |
| Best for | Admin UIs, reports | Feeds, infinite scroll |

**Use offset when:**
- Page numbers required (1, 2, 3...)
- Total count needed
- Dataset is small (< 10,000 records)
- Users jump to specific pages

**Use cursor when:**
- Dataset is large (> 100,000 records)
- Infinite scroll / "Load More" UI
- Data changes frequently during pagination
- Performance is critical

## Contributing

Contributions welcome. Open an issue or PR on GitHub.

## License

[MIT License](LICENSE.md)
