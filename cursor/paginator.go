package cursor

import (
	"context"

	"github.com/nrfta/paging-go/v2"
)

// PageArgs represents pagination arguments.
type PageArgs interface {
	GetFirst() *int
	GetAfter() *string
	GetSortBy() []paging.Sort
}

// Paginator implements paging.Paginator[T] for cursor-based pagination.
type Paginator[T any] struct {
	fetcher paging.Fetcher[T]
	schema  *Schema[T]
}

// New creates a cursor paginator that implements paging.Paginator[T].
//
// Example:
//
//	fetcher := sqlboiler.NewFetcher(queryFunc, countFunc, sqlboiler.CursorToQueryMods)
//	paginator := cursor.New(fetcher, schema)
//	result, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
func New[T any](fetcher paging.Fetcher[T], schema *Schema[T]) paging.Paginator[T] {
	return &Paginator[T]{
		fetcher: fetcher,
		schema:  schema,
	}
}

// Paginate executes cursor-based pagination and returns a Page[T].
func (p *Paginator[T]) Paginate(
	ctx context.Context,
	args *paging.PageArgs,
	opts ...paging.PaginateOption,
) (*paging.Page[T], error) {
	// Apply page size config
	pageConfig := paging.ApplyPaginateOptions(args, opts...)
	limit := pageConfig.EffectiveLimit(args)

	// Get encoder for current sort
	encoder, err := p.schema.EncoderFor(args)
	if err != nil {
		return nil, err
	}

	// Decode cursor
	var cursorPos *paging.CursorPosition
	if args != nil && args.GetAfter() != nil {
		cursorPos, _ = encoder.Decode(*args.GetAfter())
	}

	// Build ORDER BY
	orderBy := p.schema.BuildOrderBy(getSortBy(args))

	// Fetch with N+1 pattern
	params := paging.FetchParams{
		Limit:   limit + 1,
		Cursor:  cursorPos,
		OrderBy: orderBy,
	}
	items, err := p.fetcher.Fetch(ctx, params)
	if err != nil {
		return nil, err
	}

	// Detect hasNextPage and trim
	hasNextPage := len(items) > limit
	if hasNextPage {
		items = items[:limit]
	}

	// Build PageInfo
	pageInfo := buildCursorPageInfo(encoder, items, cursorPos, hasNextPage)

	return &paging.Page[T]{
		Nodes:    items,
		PageInfo: &pageInfo,
		Metadata: paging.Metadata{Strategy: "cursor"},
	}, nil
}

// getSortBy safely extracts SortBy from args.
func getSortBy(args *paging.PageArgs) []paging.Sort {
	if args == nil || args.GetSortBy() == nil {
		return nil
	}
	return args.GetSortBy()
}

// buildCursorPageInfo creates PageInfo for cursor-based pagination.
func buildCursorPageInfo[T any](
	encoder paging.CursorEncoder[T],
	items []T,
	currentCursor *paging.CursorPosition,
	hasNextPage bool,
) paging.PageInfo {
	return paging.PageInfo{
		TotalCount: func() (*int, error) {
			return nil, nil
		},

		StartCursor: func() (*string, error) {
			if len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[0])
		},

		EndCursor: func() (*string, error) {
			if len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[len(items)-1])
		},

		HasNextPage: func() (bool, error) {
			return hasNextPage, nil
		},

		HasPreviousPage: func() (bool, error) {
			return currentCursor != nil, nil
		},
	}
}

// BuildConnection transforms a Page[From] to a Connection[To] for GraphQL.
// Uses schema's encoder to generate composite key cursors.
//
// Example:
//
//	result, _ := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
//	conn, _ := cursor.BuildConnection(result, schema, toDomainUser)
func BuildConnection[From any, To any](
	page *paging.Page[From],
	schema *Schema[From],
	args *paging.PageArgs,
	transform func(From) (To, error),
) (*paging.Connection[To], error) {
	encoder, err := schema.EncoderFor(args)
	if err != nil {
		return nil, err
	}

	return paging.BuildConnection(
		page.Nodes,
		*page.PageInfo,
		func(i int, item From) string {
			cursor, _ := encoder.Encode(item)
			if cursor == nil {
				return ""
			}
			return *cursor
		},
		transform,
	)
}
