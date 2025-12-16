// Package offset provides offset-based pagination functionality.
//
// This package implements traditional offset/limit pagination with support for
// sorting and cursor encoding. It implements the paging.Paginator[T] interface
// and works with the Fetcher pattern for database abstraction.
//
// Example usage:
//
//	fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.OffsetToQueryMods)
//	paginator := offset.New(fetcher)
//	result, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
//	conn, err := offset.BuildConnection(result, toDomainModel)
package offset

import (
	"context"

	"github.com/nrfta/paging-go/v2"
)

// PageArgs represents pagination arguments.
// This is a subset of the main PageArgs type to avoid import cycles.
// Implementations should provide the page size (First), cursor position (After),
// and sorting configuration (SortBy).
type PageArgs interface {
	GetFirst() *int
	GetAfter() *string
	GetSortBy() []paging.Sort
}

// Paginator implements paging.Paginator[T] for offset-based pagination.
// It wraps a Fetcher[T] and handles limit/offset calculation, ordering,
// and page metadata generation.
type Paginator[T any] struct {
	fetcher paging.Fetcher[T]
}

// New creates an offset paginator that implements paging.Paginator[T].
// Takes a Fetcher[T] which handles database queries.
//
// The paginator is reusable across multiple requests - each Paginate() call
// can have different page size limits.
//
// Example:
//
//	fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.OffsetToQueryMods)
//	paginator := offset.New(fetcher)
//	result, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
func New[T any](fetcher paging.Fetcher[T]) paging.Paginator[T] {
	return &Paginator[T]{fetcher: fetcher}
}

// Paginate executes offset-based pagination and returns a Page[T].
//
// The method:
//  1. Applies page size configuration from options (WithMaxSize, WithDefaultSize)
//  2. Calculates offset from cursor
//  3. Builds ORDER BY clause from sort directives
//  4. Fetches total count
//  5. Fetches items with limit/offset
//  6. Returns Page[T] with items, PageInfo, and metadata
//
// Options:
//   - WithMaxSize(n): Cap page size to maximum of n
//   - WithDefaultSize(n): Use n as default when First is nil
//
// Example:
//
//	result, err := paginator.Paginate(ctx, args,
//	    paging.WithMaxSize(1000),
//	    paging.WithDefaultSize(50),
//	)
func (p *Paginator[T]) Paginate(
	ctx context.Context,
	args *paging.PageArgs,
	opts ...paging.PaginateOption,
) (*paging.Page[T], error) {
	// Apply page size config from options
	pageConfig := paging.ApplyPaginateOptions(args, opts...)
	limit := pageConfig.EffectiveLimit(args)

	// Calculate offset from cursor
	offset := 0
	if args != nil && args.GetAfter() != nil {
		offset = DecodeCursor(args.GetAfter())
	}

	// Build ORDER BY clause
	orderBy := buildOrderBy(args)

	// Get total count
	totalCount, err := p.fetcher.Count(ctx, paging.FetchParams{})
	if err != nil {
		return nil, err
	}

	// Fetch items
	params := paging.FetchParams{
		Limit:   limit,
		Offset:  offset,
		OrderBy: orderBy,
	}
	items, err := p.fetcher.Fetch(ctx, params)
	if err != nil {
		return nil, err
	}

	// Build PageInfo
	pageInfo := buildOffsetPageInfo(limit, totalCount, offset)

	return &paging.Page[T]{
		Nodes:    items,
		PageInfo: &pageInfo,
		Metadata: paging.Metadata{
			Strategy:    "offset",
			QueryTimeMs: 0, // TODO: track timing
			Offset:      offset,
		},
	}, nil
}

// buildOrderBy constructs the ORDER BY directives from PageArgs.
// Defaults to "created_at" if no sort is specified.
func buildOrderBy(args *paging.PageArgs) []paging.Sort {
	if args == nil || args.GetSortBy() == nil || len(args.GetSortBy()) == 0 {
		return []paging.Sort{{Column: "created_at", Desc: false}}
	}
	return args.GetSortBy()
}

// buildOffsetPageInfo creates PageInfo for offset-based pagination.
// It calculates page boundaries and provides functions to query pagination state.
//
// The endOffset calculation ensures the last page cursor points to the start
// of the final complete page of results.
func buildOffsetPageInfo(
	pageSize int,
	totalCount int64,
	currentOffset int,
) paging.PageInfo {
	count := int(totalCount)
	endOffset := count - (count % pageSize)

	if endOffset == count {
		endOffset = count - pageSize
	}
	if endOffset < 0 {
		endOffset = 0
	}

	return paging.PageInfo{
		TotalCount:      func() (*int, error) { return &count, nil },
		StartCursor:     func() (*string, error) { return EncodeCursor(0), nil },
		EndCursor:       func() (*string, error) { return EncodeCursor(endOffset), nil },
		HasNextPage:     func() (bool, error) { return (currentOffset + pageSize) < count, nil },
		HasPreviousPage: func() (bool, error) { return currentOffset > 0, nil },
	}
}

// BuildConnection transforms a Page[From] to a Connection[To] for GraphQL.
// It handles transformation from database models to domain models and generates
// sequential offset-based cursors for each item.
//
// This function eliminates the manual boilerplate of building edges and nodes arrays.
//
// Type parameters:
//   - From: Source type (e.g., *models.User from SQLBoiler)
//   - To: Target type (e.g., *domain.User for GraphQL)
//
// Parameters:
//   - page: The Page[From] returned from Paginate()
//   - transform: Function that converts database model to domain model
//
// Returns a Connection with edges, nodes, and pageInfo populated.
//
// Example:
//
//	result, _ := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
//	conn, _ := offset.BuildConnection(result, toDomainUser)
func BuildConnection[From any, To any](
	page *paging.Page[From],
	transform func(From) (To, error),
) (*paging.Connection[To], error) {
	// Get starting offset from page metadata
	// This allows correct cursor generation for pages beyond the first one
	startOffset := page.Metadata.Offset

	return paging.BuildConnection(
		page.Nodes,
		*page.PageInfo,
		func(i int, _ From) string {
			cursor := EncodeCursor(startOffset + i + 1)
			if cursor == nil {
				return ""
			}
			return *cursor
		},
		transform,
	)
}
