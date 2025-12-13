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

	"github.com/nrfta/go-paging"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
)

const defaultLimitVal = 50

// PageArgs represents pagination arguments.
// This is a subset of the main PageArgs type to avoid import cycles.
// Implementations should provide the page size (First), cursor position (After),
// and sorting configuration (SortBy).
type PageArgs interface {
	GetFirst() *int
	GetAfter() *string
	GetSortBy() []paging.OrderBy
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
	if page != nil && len(page.GetSortBy()) > 0 {
		// Build ORDER BY from sort specifications
		var parts []string
		for _, sort := range page.GetSortBy() {
			part := sort.Column
			if sort.Desc {
				part += " DESC"
			}
			parts = append(parts, part)
		}
		orderBy = strings.Join(parts, ", ")
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

// BuildConnection creates a Relay-compliant GraphQL connection from a slice of items.
// It handles transformation from database models to domain models and automatically
// generates sequential offset-based cursors for each item.
//
// This function eliminates the manual boilerplate of building edges and nodes arrays,
// reducing repository code by 60-80%.
//
// Type parameters:
//   - From: Source type (e.g., *models.User from SQLBoiler)
//   - To: Target type (e.g., *domain.User for GraphQL)
//
// Parameters:
//   - paginator: The offset paginator containing pagination state
//   - items: Slice of database records to transform
//   - transform: Function that converts database model to domain model
//
// Returns a Connection with edges, nodes, and pageInfo populated.
//
// Example usage:
//
//	// Before (manual boilerplate - 25+ lines):
//	result := &domain.UserConnection{PageInfo: &paginator.PageInfo}
//	for i, row := range dbUsers {
//	    user, err := toDomainUser(row)
//	    if err != nil { return nil, err }
//	    result.Edges = append(result.Edges, domain.Edge{
//	        Cursor: *offset.EncodeCursor(paginator.Offset + i + 1),
//	        Node:   user,
//	    })
//	    result.Nodes = append(result.Nodes, user)
//	}
//
//	// After (using BuildConnection - 1 line):
//	return offset.BuildConnection(paginator, dbUsers, toDomainUser)
func BuildConnection[From any, To any](
	paginator Paginator,
	items []From,
	transform func(From) (To, error),
) (*paging.Connection[To], error) {
	return paging.BuildConnection(
		items,
		paginator.PageInfo,
		func(i int, _ From) string {
			return *EncodeCursor(paginator.Offset + i + 1)
		},
		transform,
	)
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
		HasPreviousPage: func() (bool, error) { return currentOffset > 0, nil },
	}
}
