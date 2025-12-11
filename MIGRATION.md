# Migration Guide

This guide helps you migrate from the old `go-paging` API to the new modular architecture.

## Quick Summary

**What you need to change:**

1. Add import: `"github.com/nrfta/go-paging/offset"`
2. Change: `paging.NewOffsetPaginator(...)` → `offset.New(...)`
3. Change: `paging.OffsetPaginator` → `offset.Paginator`

**What stays the same:**

- `paging.PageArgs` usage
- `paging.PageInfo` type
- `QueryMods()` method
- SQLBoiler integration

## Overview

The library has been refactored to use a modular package structure:

- **`offset/`** package: Contains offset-based pagination implementation with cursor encoding
- **Root package**: Contains shared types (`PageArgs`, `PageInfo`) used across all pagination strategies

## Breaking Changes

### Removed: `paging.NewOffsetPaginator()`

The `NewOffsetPaginator()` function and `OffsetPaginator` type have been removed from the root package.

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

### 4. PageInfo Access (No Changes Required!)

**Good news:** PageInfo usage is identical! The `offset.Paginator.PageInfo` field is actually `paging.PageInfo` - no conversion needed.

```go
// Works exactly the same as before
pageInfo := paginator.PageInfo

// Or use the helper method
pageInfo := paginator.GetPageInfo()

// Both return paging.PageInfo directly
```

## Complete Example

### Before (Old API)

```go
package main

import (
    "github.com/nrfta/go-paging"
)

func GetItems(pageArgs *paging.PageArgs, db *sql.DB) ([]Item, paging.PageInfo, error) {
    // Get total count
    totalCount := getItemCount(db)

    // Create paginator
    paginator := paging.NewOffsetPaginator(pageArgs, totalCount)

    // Use query mods
    items, err := models.Items(paginator.QueryMods()...).All(ctx, db)
    if err != nil {
        return nil, paging.PageInfo{}, err
    }

    return items, paginator.PageInfo, nil
}
```

### After (New API)

```go
package main

import (
    "github.com/nrfta/go-paging"
    "github.com/nrfta/go-paging/offset"
)

func GetItems(pageArgs *paging.PageArgs, db *sql.DB) ([]Item, paging.PageInfo, error) {
    // Get total count
    totalCount := getItemCount(db)

    // Create paginator with new API
    paginator := offset.New(pageArgs, totalCount)

    // Use query mods (same as before)
    items, err := models.Items(paginator.QueryMods()...).All(ctx, db)
    if err != nil {
        return nil, paging.PageInfo{}, err
    }

    // PageInfo works directly - no conversion needed!
    return items, paginator.PageInfo, nil
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

**Note:** Most users don't need to call these functions directly - the paginator handles cursor encoding/decoding automatically. You only need these if you're manually building cursors for testing or custom pagination logic.

## Why This Change?

The new modular architecture provides:

1. **Clearer separation of concerns**: Each pagination strategy in its own package
2. **Easier to extend**: New pagination strategies (cursor-based, quota-fill) can be added without conflicts
3. **Better documentation**: Each package can be documented independently
4. **More flexible**: Paginator implementations are self-contained

## Need Help?

If you encounter issues during migration, please:

1. Check this guide thoroughly
2. Review the examples in the repository
3. Open an issue on GitHub with your specific use case
