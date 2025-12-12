# go-paging ![](https://github.com/nrfta/go-paging/workflows/CI/badge.svg)

Modern Go pagination for [SQLBoiler](https://github.com/aarondl/sqlboiler) and [gqlgen](https://github.com/99designs/gqlgen/) (GraphQL).

**New in v2:** Built-in Connection/Edge builders eliminate 60-80% of boilerplate code.

## Features

- **Relay-compliant GraphQL pagination** with automatic Connection/Edge building
- **Type-safe transformations** from database models to domain models
- **Modular architecture** with pluggable pagination strategies
- **SQLBoiler integration** with query mod support
- **Extensible** - Ready for cursor-based and quota-fill pagination (coming soon)

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

## Before/After Comparison

### Before v2 (Manual Boilerplate)

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*UserConnection, error) {
 totalCount, _ := models.Users().Count(ctx, r.DB)
 paginator := paging.NewOffsetPaginator(page, totalCount)

 dbUsers, _ := models.Users(paginator.QueryMods()...).All(ctx, r.DB)

 // Manual boilerplate - 15+ lines
 result := &UserConnection{PageInfo: &paginator.PageInfo}
 for i, row := range dbUsers {
  user, err := toDomainUser(row)
  if err != nil {
   return nil, err
  }
  result.Edges = append(result.Edges, &UserEdge{
   Cursor: *paging.EncodeOffsetCursor(paginator.Offset + i + 1),
   Node:   user,
  })
  result.Nodes = append(result.Nodes, user)
 }
 return result, nil
}
```

### After v2 (BuildConnection)

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[User], error) {
 totalCount, _ := models.Users().Count(ctx, r.DB)
 paginator := offset.New(page, totalCount)
 dbUsers, _ := models.Users(paginator.QueryMods()...).All(ctx, r.DB)

 // One line - library handles everything
 return offset.BuildConnection(paginator, dbUsers, toDomainUser)
}
```

**Result:** 60-80% less code, no manual cursor encoding, automatic error handling.

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

## Architecture

```
go-paging/
├── connection.go          # Generic Connection[T] and Edge[T] types
├── interfaces.go          # Core pagination interfaces (Paginator[T], Fetcher[T])
├── models.go              # PageArgs and PageInfo
├── offset/                # Offset-based pagination
│   ├── paginator.go       # Offset paginator + BuildConnection
│   └── cursor.go          # Offset cursor encoding
└── sqlboiler/             # SQLBoiler ORM adapter
    ├── fetcher.go         # Generic Fetcher[T] (ORM integration)
    ├── offset.go          # Offset query builder (strategy-specific)
    └── cursor.go          # Cursor query builder (Phase 2)
```

**Design Philosophy:**

- **ORM adapters** (sqlboiler, gorm, etc.) are generic and strategy-agnostic
- **Query builders** (offset, cursor) are strategy-specific
- **Easy to extend:** Add new strategies without changing ORM adapters

## Roadmap

**Phase 1 (Current - v2.0):** ✅

- Connection/Edge builders
- Modular architecture
- Offset pagination

**Phase 2 (Coming Soon):**

- Cursor-based pagination (keyset pagination)
- High-performance for large datasets
- O(1) complexity regardless of page depth

**Phase 3 (Planned):**

- Quota-fill pagination wrapper
- Authorization-aware filtering
- Consistent page sizes with per-item filtering

## Contributing

Contributions welcome! Please open an issue or PR.

## License

This project is licensed under the [MIT License](LICENSE.md).
