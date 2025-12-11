// Package offset provides offset-based pagination functionality.
//
// This package implements traditional offset/limit pagination with support for
// sorting and cursor encoding. It is designed to work with SQLBoiler query mods
// and provides a clean interface for paginating database results.
//
// Example usage:
//
//	paginator := offset.New(pageArgs, totalCount)
//	mods := paginator.QueryMods()
//	results, err := models.Items(mods...).All(ctx, db)
package offset

import (
	"strings"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
)

const defaultLimitVal = 50

// PageArgs represents pagination arguments.
// This is a subset of the main PageArgs type to avoid import cycles.
// Implementations should provide the page size (First), cursor position (After),
// and sorting configuration (SortByCols, IsDesc).
type PageArgs interface {
	GetFirst() *int
	GetAfter() *string
	SortByCols() []string
	IsDesc() bool
}

// Paginator is the paginator for offset-based pagination.
// It encapsulates limit, offset, and page metadata for database queries.
type Paginator struct {
	Limit    int
	Offset   int
	PageInfo paging.PageInfo
	orderBy  string
}

// New creates a new offset paginator.
//
// Parameters:
//   - page: Pagination arguments including page size, cursor, and sorting
//   - totalCount: Total number of records available
//   - defaultLimit: Optional default page size (defaults to 50 if not provided)
//
// The paginator automatically handles:
//   - Default page size of 50 records
//   - Zero-value protection to prevent divide-by-zero errors
//   - Sorting with default "created_at" column
//   - Descending order when specified
//   - Cursor encoding/decoding using base64
func New(
	page PageArgs,
	totalCount int64,
	defaultLimit ...*int,
) Paginator {
	limit := defaultLimitVal
	if len(defaultLimit) > 0 && defaultLimit[0] != nil {
		limit = *defaultLimit[0]
	}

	if page != nil && page.GetFirst() != nil && *page.GetFirst() > 0 {
		limit = *page.GetFirst()
	}

	// Ensure limit is never 0 to avoid divide by zero
	if limit == 0 {
		limit = defaultLimitVal
	}

	var offset int
	if page != nil {
		offset = DecodeCursor(page.GetAfter())
	}

	orderBy := "created_at"
	if page != nil && len(page.SortByCols()) > 0 {
		orderBy = strings.Join(page.SortByCols(), ", ")
	}

	if page != nil && page.IsDesc() {
		orderBy = orderBy + " DESC"
	}

	pageInfo := newOffsetBasedPageInfo(&limit, totalCount, offset)

	return Paginator{
		Limit:    limit,
		Offset:   offset,
		PageInfo: pageInfo,
		orderBy:  orderBy,
	}
}

// QueryMods returns SQLBoiler query modifiers for pagination.
// These mods apply offset, limit, and order by clauses to a query.
//
// Example usage:
//
//	items, err := models.Items(paginator.QueryMods()...).All(ctx, db)
func (p *Paginator) QueryMods() []qm.QueryMod {
	return []qm.QueryMod{
		qm.Offset(p.Offset),
		qm.Limit(p.Limit),
		qm.OrderBy(p.orderBy),
	}
}

// GetPageInfo returns the PageInfo for this paginator.
// PageInfo contains functions to retrieve pagination metadata like
// total count, cursors, and whether next/previous pages exist.
func (p *Paginator) GetPageInfo() paging.PageInfo {
	return p.PageInfo
}

// GetOrderBy returns the ORDER BY clause used by this paginator.
// This includes the column names and DESC modifier if applicable.
func (p *Paginator) GetOrderBy() string {
	return p.orderBy
}

// newOffsetBasedPageInfo creates PageInfo for offset-based pagination.
// It calculates page boundaries and provides functions to query pagination state.
//
// The endOffset calculation ensures the last page cursor points to the start
// of the final complete page of results.
func newOffsetBasedPageInfo(
	pageSize *int,
	totalCount int64,
	currentOffset int,
) paging.PageInfo {
	count := int(totalCount)
	endOffset := count - (count % *pageSize)

	if endOffset == count {
		endOffset = count - *pageSize
	}

	return paging.PageInfo{
		TotalCount:      func() (*int, error) { return &count, nil },
		StartCursor:     func() (*string, error) { return EncodeCursor(0), nil },
		EndCursor:       func() (*string, error) { return EncodeCursor(endOffset), nil },
		HasNextPage:     func() (bool, error) { return (currentOffset+*pageSize < count), nil },
		HasPreviousPage: func() (bool, error) { return (currentOffset-*pageSize > 0), nil },
	}
}
