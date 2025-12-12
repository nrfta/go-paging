# Migration Guide

This guide helps you migrate from the old `go-paging` API to the new modular architecture.

## Quick Summary

**What you need to change:**

1. Add import: `"github.com/nrfta/go-paging/offset"`
2. Change: `paging.NewOffsetPaginator(...)` → `offset.New(...)`
3. Change: `paging.OffsetPaginator` → `offset.Paginator`
4. **New (Recommended):** Use `offset.BuildConnection()` to eliminate boilerplate

**What stays the same:**

- `paging.PageArgs` usage
- `paging.PageInfo` type
- `QueryMods()` method
- SQLBoiler integration

**What you gain:**

- ✨ **60-80% less boilerplate** with `offset.BuildConnection()`
- ✨ Generic `Connection[T]` and `Edge[T]` types
- ✨ Type-safe transformations with automatic error handling
- ✨ Modular architecture ready for cursor and quota-fill pagination

## Overview

The library has been refactored to use a modular package structure:

- **`offset/`** package: Offset-based pagination with cursor encoding
- **`sqlboiler/`** package: SQLBoiler ORM adapter (generic + strategy-specific)
- **Root package**: Shared types (`PageArgs`, `PageInfo`, `Connection[T]`, `Edge[T]`)

## Breaking Changes

### Removed: `paging.NewOffsetPaginator()`

The `NewOffsetPaginator()` function and `OffsetPaginator` type have been removed from the root package.

### Removed: `paging.EncodeOffsetCursor()` / `paging.DecodeOffsetCursor()`

Cursor functions moved to `offset` package (see [Cursor Functions](#cursor-functions)).

## Migration Steps

### 1. Update Your Imports

Add the offset package import:

```go
import (
    "github.com/nrfta/go-paging"
    "github.com/nrfta/go-paging/offset"  // Add this
)
```

### 2. Update Paginator Creation

**Before:**

```go
paginator := paging.NewOffsetPaginator(pageArgs, totalCount)
```

**After:**

```go
paginator := offset.New(pageArgs, totalCount)
```

**With custom default limit:**

```go
// Before
defaultLimit := 100
paginator := paging.NewOffsetPaginator(pageArgs, totalCount, &defaultLimit)

// After
defaultLimit := 100
paginator := offset.New(pageArgs, totalCount, &defaultLimit)
```

### 3. Update Type References

**Before:**

```go
var paginator paging.OffsetPaginator
```

**After:**

```go
var paginator offset.Paginator
```

### 4. Use BuildConnection (Recommended!)

This is the **biggest improvement** in v2. Instead of manually building edges and nodes, use `offset.BuildConnection()`:

**Before (Manual Boilerplate - 15+ lines):**

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*UserConnection, error) {
    totalCount, _ := models.Users().Count(ctx, r.DB)
    paginator := paging.NewOffsetPaginator(page, totalCount)

    dbUsers, _ := models.Users(paginator.QueryMods()...).All(ctx, r.DB)

    // Manual boilerplate
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

**After (BuildConnection - 1 line!):**

```go
func (r *queryResolver) Users(ctx context.Context, page *paging.PageArgs) (*paging.Connection[User], error) {
    totalCount, _ := models.Users().Count(ctx, r.DB)
    paginator := offset.New(page, totalCount)

    dbUsers, _ := models.Users(paginator.QueryMods()...).All(ctx, r.DB)

    // One line - library handles everything!
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

**Benefits:**
- ✅ 60-80% less code
- ✅ No manual cursor encoding
- ✅ Automatic error handling
- ✅ Type-safe transformations
- ✅ Works with both `edges` and `nodes` fields

### 5. PageInfo Access (No Changes Required!)

**Good news:** PageInfo usage is identical! The `offset.Paginator.PageInfo` field is `paging.PageInfo` - no conversion needed.

```go
// Works exactly the same as before
pageInfo := paginator.PageInfo

// Or use the helper method
pageInfo := paginator.GetPageInfo()

// Both return paging.PageInfo directly
```

## Complete Example

### Before (Old API - Manual Boilerplate)

```go
package main

import (
    "context"
    "database/sql"

    "github.com/nrfta/go-paging"
    "github.com/my-user/my-app/models"
)

type UserConnection struct {
    Edges    []*UserEdge
    PageInfo *paging.PageInfo
}

type UserEdge struct {
    Cursor string
    Node   *models.User
}

func GetUsers(ctx context.Context, pageArgs *paging.PageArgs, db *sql.DB) (*UserConnection, error) {
    // Get total count
    totalCount, err := models.Users().Count(ctx, db)
    if err != nil {
        return nil, err
    }

    // Create paginator
    paginator := paging.NewOffsetPaginator(pageArgs, totalCount)

    // Fetch records
    dbUsers, err := models.Users(paginator.QueryMods()...).All(ctx, db)
    if err != nil {
        return nil, err
    }

    // Manual boilerplate - 15+ lines
    result := &UserConnection{PageInfo: &paginator.PageInfo}
    for i, row := range dbUsers {
        result.Edges = append(result.Edges, &UserEdge{
            Cursor: *paging.EncodeOffsetCursor(paginator.Offset + i + 1),
            Node:   row,
        })
    }

    return result, nil
}
```

### After (New API - BuildConnection)

```go
package main

import (
    "context"
    "database/sql"

    "github.com/nrfta/go-paging"
    "github.com/nrfta/go-paging/offset"
    "github.com/my-user/my-app/models"
)

func GetUsers(ctx context.Context, pageArgs *paging.PageArgs, db *sql.DB) (*paging.Connection[models.User], error) {
    // Get total count
    totalCount, err := models.Users().Count(ctx, db)
    if err != nil {
        return nil, err
    }

    // Create paginator
    paginator := offset.New(pageArgs, totalCount)

    // Fetch records
    dbUsers, err := models.Users(paginator.QueryMods()...).All(ctx, db)
    if err != nil {
        return nil, err
    }

    // One line - automatic edge/node building with identity transform
    return offset.BuildConnection(paginator, dbUsers, func(u *models.User) (*models.User, error) {
        return u, nil  // No transformation needed
    })
}
```

**With transformation (database model → domain model):**

```go
type DomainUser struct {
    ID       string
    FullName string
}

func GetUsers(ctx context.Context, pageArgs *paging.PageArgs, db *sql.DB) (*paging.Connection[DomainUser], error) {
    totalCount, _ := models.Users().Count(ctx, db)
    paginator := offset.New(pageArgs, totalCount)
    dbUsers, _ := models.Users(paginator.QueryMods()...).All(ctx, db)

    // Automatic transformation with error handling
    return offset.BuildConnection(paginator, dbUsers, func(db *models.User) (*DomainUser, error) {
        return &DomainUser{
            ID:       db.ID,
            FullName: db.Name,
        }, nil
    })
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

| Old (v1) | New (v2) |
|----------|----------|
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

## Advanced: Generic Connection Types

v2 introduces generic `Connection[T]` and `Edge[T]` types:

```go
// Built-in generic types
type Connection[T any] struct {
    Edges    []Edge[T]
    Nodes    []*T
    PageInfo PageInfo
}

type Edge[T any] struct {
    Cursor string
    Node   *T
}
```

**GraphQL Schema:**

```graphql
# Use these built-in types with gqlgen
type UserConnection {
  edges: [UserEdge!]!
  nodes: [User!]!
  pageInfo: PageInfo!
}

type UserEdge {
  cursor: String!
  node: User!
}
```

**gqlgen.yml:**

```yaml
models:
  UserConnection:
    model: github.com/nrfta/go-paging.Connection[github.com/my-user/my-app/domain.User]
```

## Advanced: SQLBoiler Adapter (for library authors)

The SQLBoiler adapter has been refactored for extensibility. **Most users don't need to change anything** - this only affects advanced use cases.

**What changed:**
- Split into `fetcher.go` (generic ORM integration) + `offset.go` (strategy-specific queries)
- Enables future support for cursor pagination and other ORMs (GORM, sqlc, etc.)

**If you were using internal SQLBoiler functions directly:**

```go
// Before
mods := sqlboiler.ToQueryMods(params)

// After
mods := sqlboiler.OffsetToQueryMods(params)
```

## Why This Change?

The new modular architecture provides:

1. **Eliminates boilerplate**: `BuildConnection()` reduces resolver code by 60-80%
2. **Type-safe transformations**: Generic transform functions with automatic error handling
3. **Clearer separation of concerns**: Each pagination strategy in its own package
4. **Easier to extend**: New strategies (cursor-based, quota-fill) can be added without conflicts
5. **ORM flexibility**: Easy to add support for GORM, sqlc, or custom ORMs
6. **Better documentation**: Each package documented independently
7. **Production-ready**: Comprehensive tests, optimized code, clear patterns

## Migration Checklist

- [ ] Update imports to include `"github.com/nrfta/go-paging/offset"`
- [ ] Replace `paging.NewOffsetPaginator()` with `offset.New()`
- [ ] Update type references from `paging.OffsetPaginator` to `offset.Paginator`
- [ ] Replace manual edge/node building with `offset.BuildConnection()`
- [ ] Update cursor function calls (if used): `paging.EncodeOffsetCursor` → `offset.EncodeCursor`
- [ ] Run tests to verify everything works
- [ ] Enjoy 60-80% less boilerplate code! 🎉

## Need Help?

If you encounter issues during migration, please:

1. Check this guide thoroughly
2. Review the [README.md](./README.md) for examples
3. Check the test files for usage patterns
4. Open an issue on GitHub with your specific use case

## Coming Soon

**Phase 2: Cursor Pagination**
- High-performance keyset pagination
- O(1) complexity regardless of page depth
- Same `BuildConnection()` pattern

**Phase 3: Quota-Fill Pagination**
- Authorization-aware filtering
- Consistent page sizes with per-item filtering
- Works with cursor or offset strategies
