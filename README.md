# go-paging ![](https://github.com/nrfta/go-paging/workflows/CI/badge.svg)

Modern Go pagination for [SQLBoiler](https://github.com/aarondl/sqlboiler) and [gqlgen](https://github.com/99designs/gqlgen/) (GraphQL).

I built this after struggling with the repetitive boilerplate of implementing Relay-compliant pagination across dozens of GraphQL resolvers. Each resolver needed 15-20 lines of identical edge-building, cursor-encoding, and error-handling code. When we started hitting performance issues with offset pagination on tables with millions of records, adding cursor pagination would have meant duplicating all that boilerplate again with a different strategy.

So I designed go-paging with three core ideas: eliminate the boilerplate with type-safe builders, make pagination strategies pluggable and composable, and handle the hard parts (cursor encoding, N+1 detection, filter-aware fetching) automatically. The result is a library where changing pagination strategies is a matter of swapping imports rather than rewriting logic.

**What's new in v1.0:** Built-in Connection/Edge builders eliminate 60-80% of boilerplate code. Complete support for offset, cursor, and quota-fill pagination strategies with a modular architecture that makes each strategy independent and composable.

## Features

- **Relay-compliant GraphQL pagination** with automatic Connection/Edge building
- **Type-safe transformations** from database models to domain models
- **Three composable pagination strategies:**
  - Offset pagination - Traditional LIMIT/OFFSET with page numbers
  - Cursor pagination - High-performance keyset pagination (O(1) lookups)
  - Quota-fill pagination - Filter-aware iterative fetching for authorization and soft-deletes
- **SQLBoiler integration** with query mod support
- **BuildConnection() helpers** eliminate 60-80% of boilerplate

## Install

```sh
go get -u "github.com/nrfta/go-paging"
```

## Migration from v0.3.0

If you're upgrading from v0.3.0, see [MIGRATION.md](./MIGRATION.md) for detailed migration steps. The core change is moving from a monolithic API to a modular package structure with strategy-specific imports.

**Quick summary:**

1. Add import: `"github.com/nrfta/go-paging/offset"`
2. Change: `paging.NewOffsetPaginator()` → `offset.New()`
3. Use: `offset.BuildConnection()` to eliminate boilerplate

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

This example uses offset pagination, the simplest entry point for most applications.

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

**Resolver:**

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

 // Build connection with automatic edge/node creation
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

That's it. No manual loops, no cursor encoding, no edge building boilerplate. The library handles everything automatically.

## Pagination Strategies

go-paging provides three pagination strategies that can be used independently or composed together.

### Offset Pagination

Traditional LIMIT/OFFSET pagination with page numbers. Best for small-to-medium datasets where users need to jump to specific pages.

**When to use:**

- Admin UIs with page numbers
- Reports and exports
- Datasets under 10,000 records
- Users need total count

**Performance characteristics:**

- Page 1: Fast (5ms)
- Page 1000: Slow (1000ms+) - database must scan all preceding rows

See the Quick Start example above for basic usage.

**Advanced: Custom page size and sorting**

```go
// Custom default limit
defaultLimit := 25
paginator := offset.New(pageArgs, totalCount, &defaultLimit)

// Sort by created_at descending
pageArgs := paging.WithSortBy(nil, true, "created_at")

// Sort by multiple columns
pageArgs := paging.WithSortBy(nil, true, "created_at", "id")
```

### Cursor Pagination

High-performance keyset pagination for large datasets. Uses composite indexes instead of offset scanning.

**When to use:**

- Large datasets (100,000+ records)
- Infinite scroll / "Load More" UIs
- Real-time feeds
- Performance is critical

**Performance characteristics:**

- Page 1: Fast (5ms)
- Page 1,000,000: Fast (5ms) - O(1) performance regardless of depth

**Basic example:**

```go
import (
 "github.com/nrfta/go-paging/cursor"
 "github.com/nrfta/go-paging/sqlboiler"
)

func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[User], error) {
 // 1. Create encoder (defines cursor structure)
 encoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
  return map[string]any{
   "created_at": u.CreatedAt,
   "id":         u.ID,
  }
 })

 // 2. Create fetcher with cursor strategy
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

 // 3. Build fetch params with automatic N+1
 orderBy := []paging.OrderBy{
  {Column: "created_at", Desc: true},
  {Column: "id", Desc: true},
 }
 fetchParams := cursor.BuildFetchParams(page, encoder, orderBy)

 // 4. Fetch data
 users, err := fetcher.Fetch(ctx, fetchParams)
 if err != nil {
  return nil, err
 }

 // 5. Build connection (automatically trims to requested limit)
 paginator := cursor.New(page, encoder, users)
 return cursor.BuildConnection(paginator, users, encoder, toDomainUser)
}
```

**Critical: ORDER BY rules**

When using cursor pagination, ORDER BY clauses must be defined in `FetchParams.OrderBy`, not in your query mods. Adding `qm.OrderBy()` to your fetcher function will cause duplicate records and incorrect results.

**Why?** Cursor pagination generates WHERE clauses based on the sort columns:

```sql
WHERE (created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC
```

If you add `qm.OrderBy("name ASC")` in the fetcher, the final query becomes:

```sql
WHERE (created_at < ?)  -- Filters by created_at
ORDER BY name ASC, created_at DESC, id DESC  -- But sorts by name!
```

Now the WHERE clause and ORDER BY are misaligned, causing wrong results.

**❌ WRONG - Causes duplicates:**

```go
fetcher := sqlboiler.NewFetcher(
 func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
  // ❌ DO NOT add qm.OrderBy() here
  mods = append([]qm.QueryMod{
   qm.OrderBy("name ASC"),  // ❌ Conflicts with cursor's ORDER BY
  }, mods...)
  return models.Users(mods...).All(ctx, r.DB)
 },
 countFunc,
 sqlboiler.CursorToQueryMods,
)
```

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

**Required database index:**

For optimal performance, create a composite index matching your sort columns:

```sql
-- For sorting by (created_at DESC, id DESC)
CREATE INDEX idx_users_cursor ON users(created_at DESC, id DESC);

-- For sorting by (name ASC, id ASC)
CREATE INDEX idx_users_name_cursor ON users(name ASC, id ASC);
```

**N+1 pattern:**

All three pagination strategies use the N+1 pattern internally to detect if there's a next page:

1. Fetch LIMIT + 1 records from the database
2. If you get LIMIT + 1 records, `HasNextPage = true`
3. The paginator automatically trims to LIMIT records for the response

**This is now automatic** - you don't need to manually add +1:

```go
// ✅ N+1 handled automatically!
orderBy := []paging.OrderBy{
 {Column: "created_at", Desc: true},
 {Column: "id", Desc: true},
}
fetchParams := cursor.BuildFetchParams(page, encoder, orderBy)  // Adds +1 internally
users, _ := fetcher.Fetch(ctx, fetchParams)

paginator := cursor.New(page, encoder, users)  // Detects N+1, trims to requested limit
conn, _ := cursor.BuildConnection(paginator, users, encoder, transform)
// conn.Nodes has the requested number of items, conn.PageInfo.HasNextPage() is accurate
```

### Quota-Fill Pagination

Wraps any paginator with filter-aware iterative fetching. Solves the problem of inconsistent page sizes when applying authorization filters or other per-item filtering logic.

**The problem:** Standard pagination with filtering creates inconsistent page sizes:

```go
// Request 10 items
users, _ := fetcher.Fetch(ctx, limit: 10)

// Apply authorization filter
authorized := filterAuthorized(users)  // Returns 3 items

// Problem: User asked for 10, but got 3!
return authorized  // ❌ Inconsistent page size
```

This creates poor UX: uneven layouts, "Load More" buttons that appear/disappear, users clicking multiple times to see a full page.

**The solution:** Quota-fill iteratively fetches batches until the requested page size is filled:

```go
import (
 "github.com/nrfta/go-paging/quotafill"
)

func (r *queryResolver) Organizations(ctx context.Context, page *paging.PageArgs) (*paging.Connection[Organization], error) {
 // 1. Create base paginator (cursor or offset)
 encoder := cursor.NewCompositeCursorEncoder(func(o *models.Organization) map[string]any {
  return map[string]any{"created_at": o.CreatedAt, "id": o.ID}
 })

 // 2. Create fetcher with database filters
 fetcher := sqlboiler.NewFetcher(
  func(ctx context.Context, mods ...qm.QueryMod) ([]*models.Organization, error) {
   mods = append([]qm.QueryMod{
    qm.Where("deleted_at IS NULL"),  // Pre-filter in DB
   }, mods...)
   return models.Organizations(mods...).All(ctx, r.DB)
  },
  func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
   return 0, nil
  },
  sqlboiler.CursorToQueryMods,
 )

 // 3. Fetch initial batch and create base paginator
 orderBy := []paging.OrderBy{
  {Column: "created_at", Desc: true},
  {Column: "id", Desc: true},
 }
 fetchParams := cursor.BuildFetchParams(page, encoder, orderBy)  // N+1 automatic

 orgs, err := fetcher.Fetch(ctx, fetchParams)
 if err != nil {
  return nil, err
 }

 basePaginator := cursor.New(page, encoder, orgs)

 // 4. Define authorization filter
 authFilter := func(ctx context.Context, orgs []*models.Organization) ([]*models.Organization, error) {
  return r.AuthzClient.FilterAuthorized(ctx, r.CurrentUser(ctx), orgs)
 }

 // 5. Wrap with quota-fill
 paginator := quotafill.New(basePaginator, authFilter, encoder,
  quotafill.WithMaxIterations(5),
  quotafill.WithMaxRecordsExamined(100),
 )

 // 6. Paginate with quota-fill
 result, err := paginator.Paginate(ctx, page)
 if err != nil {
  return nil, err
 }

 // 7. Log metadata for monitoring
 if result.Metadata.SafeguardHit != nil {
  log.Warnf("Quota-fill safeguard hit: %s", *result.Metadata.SafeguardHit)
 }

 // 8. Build connection
 edges := make([]*paging.Edge[Organization], len(result.Nodes))
 nodes := make([]*Organization, len(result.Nodes))
 for i, org := range result.Nodes {
  domain, err := toDomainOrg(org)
  if err != nil {
   return nil, err
  }
  cursor, _ := encoder.Encode(org)
  edges[i] = &paging.Edge[Organization]{
   Cursor: *cursor,
   Node:   domain,
  }
  nodes[i] = domain
 }

 return &paging.Connection[Organization]{
  Edges:    edges,
  Nodes:    nodes,
  PageInfo: result.PageInfo,
 }, nil
}
```

**How it works:**

1. Initialize: `filteredItems = []`, `iteration = 0`
2. Loop while `len(filteredItems) < requestedSize + 1` (N+1 pattern):
   - Calculate `fetchSize = (remaining quota) × backoffMultiplier[iteration]`
   - Fetch batch from base paginator
   - Apply filter function
   - Append filtered items to results
   - Check safeguards (maxIterations, maxRecordsExamined, timeout)
   - Break if no more data or safeguard triggered
3. Trim to `requestedSize` if we got N+1 items
4. Return results with metadata

**Adaptive backoff:**

Quota-fill uses Fibonacci-like multipliers `[1, 2, 3, 5, 8]` to optimize fetching based on filter pass rates:

- **Iteration 1:** Fetch exactly what's needed (1×)
- **Iteration 2:** Filter pass rate < 100%, overscan (2×)
- **Iteration 3+:** Progressively larger overscan (3×, 5×, 8×)

**Safeguards:**

Three configurable safeguards prevent infinite loops and excessive load:

```go
quotafill.New(paginator, filter, encoder,
 quotafill.WithMaxIterations(5),          // Default: 5
 quotafill.WithMaxRecordsExamined(100),   // Default: 100
 quotafill.WithTimeout(5 * time.Second),  // Default: 3s
)
```

When a safeguard triggers, partial results are returned with metadata:

```go
page, err := paginator.Paginate(ctx, pageArgs)

if page.Metadata.SafeguardHit != nil {
 log.Warnf("Quota-fill safeguard triggered: %s", *page.Metadata.SafeguardHit)
 log.Warnf("Returned %d items (requested %d)", len(page.Nodes), *pageArgs.First)
}
```

**Metadata tracking:**

Quota-fill tracks detailed metadata for observability:

```go
page, err := paginator.Paginate(ctx, pageArgs)

fmt.Printf("Strategy: %s\n", page.Metadata.Strategy)               // "quotafill"
fmt.Printf("Query Time: %dms\n", page.Metadata.QueryTimeMs)        // 42
fmt.Printf("Items Examined: %d\n", page.Metadata.ItemsExamined)    // 15
fmt.Printf("Iterations Used: %d\n", page.Metadata.IterationsUsed)  // 2
if page.Metadata.SafeguardHit != nil {
 fmt.Printf("Safeguard Hit: %s\n", *page.Metadata.SafeguardHit) // "max_iterations"
}
```

Use this for performance monitoring, alerting on safeguard triggers, optimizing filter pass rates, and capacity planning.

**Performance tips:**

1. **Push filtering into database queries when possible** - Reduce the amount of in-memory filtering:

```go
// ✅ Better: Pre-filter in database, quota-fill for edge cases
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

// Quota-fill only for authorization checks that can't be in SQL
authFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
 return authzClient.FilterAuthorized(ctx, currentUser, users)
}
```

2. **Monitor filter pass rates** - The higher the pass rate, the fewer iterations needed:
   - 90% pass rate: Usually 1 iteration
   - 50% pass rate: Usually 2 iterations
   - 10% pass rate: May hit safeguards

3. **Set up alerts** when safeguards trigger frequently - this indicates low filter pass rates or insufficient safeguard limits.

## Architecture

```
go-paging/
├── connection.go          # Generic Connection[T] and Edge[T] types
├── interfaces.go          # Core interfaces (Paginator[T], Fetcher[T], FilterFunc[T])
├── models.go              # PageArgs, PageInfo, Metadata
├── offset/                # Offset-based pagination
│   ├── paginator.go       # Offset paginator + BuildConnection
│   └── cursor.go          # Offset cursor encoding
├── cursor/                # Cursor-based (keyset) pagination
│   ├── paginator.go       # Cursor paginator + BuildConnection
│   └── encoder.go         # Composite cursor encoding/decoding
├── quotafill/             # Quota-fill pagination (decorator pattern)
│   └── wrapper.go         # Wraps any paginator with iterative filtering
└── sqlboiler/             # SQLBoiler ORM adapter
    ├── fetcher.go         # Generic Fetcher[T]
    ├── offset.go          # Offset query builder
    └── cursor.go          # Cursor query builder
```

The modular architecture provides clear separation of concerns:

- **ORM adapters** (sqlboiler/) are generic and strategy-agnostic
- **Pagination strategies** (offset/, cursor/) are independent packages
- **Decorators** (quotafill/) wrap any paginator to add capabilities
- **Core types** (connection.go, interfaces.go) are shared across all strategies

This makes it easy to add new pagination strategies or ORM adapters without changing existing code.

## Comparison: Offset vs Cursor

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

## Contributing

Contributions welcome. Please open an issue or PR on GitHub.

## License

This project is licensed under the [MIT License](LICENSE.md).
