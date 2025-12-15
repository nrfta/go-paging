package cursor

import (
	"fmt"

	"github.com/nrfta/paging-go/v2"
)

const defaultLimitVal = 50

// PageArgs represents pagination arguments.
// This is a subset of the main PageArgs type to avoid import cycles.
// Implementations should provide the page size (First), cursor position (After),
// and sorting configuration (SortBy).
type PageArgs interface {
	GetFirst() *int
	GetAfter() *string
	GetSortBy() []paging.Sort
}

// Paginator is the paginator for cursor-based pagination.
// It encapsulates limit, cursor position, and page metadata for keyset queries.
//
// Unlike offset pagination, cursor pagination:
//   - Does not require totalCount
//   - Uses cursor position instead of offset
//   - Provides O(1) performance regardless of page depth
//   - Requires composite indexes on sort columns
//
// Paginator now requires a Schema to enforce encoder/OrderBy matching.
type Paginator struct {
	limit    int
	cursor   *paging.CursorPosition
	PageInfo paging.PageInfo
	orderBy  []paging.Sort
	encoder  interface{} // Stored as interface{} to avoid making Paginator generic
}

// parsedParams holds the common parsed state from PageArgs and Schema.
type parsedParams[T any] struct {
	encoder paging.CursorEncoder[T]
	orderBy []paging.Sort
	limit   int
	cursor  *paging.CursorPosition
}

// parsePageArgs extracts and validates common parameters from PageArgs and Schema.
func parsePageArgs[T any](page PageArgs, schema *Schema[T], defaultLimit *int) (parsedParams[T], error) {
	encoder, err := schema.EncoderFor(page)
	if err != nil {
		return parsedParams[T]{}, err
	}

	var sortBy []paging.Sort
	if page != nil && page.GetSortBy() != nil {
		sortBy = page.GetSortBy()
	}
	orderBy := schema.BuildOrderBy(sortBy)

	limit := defaultLimitVal
	if defaultLimit != nil {
		limit = *defaultLimit
	}
	if page != nil && page.GetFirst() != nil && *page.GetFirst() > 0 {
		limit = *page.GetFirst()
	}
	if limit == 0 {
		limit = defaultLimitVal
	}

	var cursor *paging.CursorPosition
	if page != nil && page.GetAfter() != nil {
		cursor, _ = encoder.Decode(*page.GetAfter())
	}

	return parsedParams[T]{
		encoder: encoder,
		orderBy: orderBy,
		limit:   limit,
		cursor:  cursor,
	}, nil
}

// New creates a new cursor paginator using a Schema.
//
// Parameters:
//   - page: Pagination arguments including page size, cursor, and sorting
//   - schema: Schema that defines sortable fields, fixed fields, and cursor encoding
//   - items: The items fetched from the database (should be LIMIT+1 for accurate HasNextPage)
//   - defaultLimit: Optional default page size (defaults to 50 if not provided)
//
// The paginator automatically handles:
//   - PageArgs validation: Returns error if sort fields are invalid
//   - Encoder/OrderBy matching: Schema guarantees they match
//   - N+1 pattern: Detects if you fetched LIMIT+1 for accurate HasNextPage detection
//   - Item trimming: Trims results to requested limit if N+1 was fetched
//   - Default page size of 50 records
//   - Zero-value protection to prevent divide-by-zero errors
//   - Cursor decoding using the schema's encoder
//   - PageInfo generation based on fetched results
//
// Best Practice (N+1 Pattern):
//   - Fetch LIMIT+1 records from database (e.g., if limit=10, fetch 11)
//   - Pass all fetched items to New()
//   - Paginator will detect N+1 and set HasNextPage accurately
//   - BuildConnection will automatically trim to LIMIT items
//
// Example usage:
//
//	// Define schema once at app startup
//	schema := cursor.NewSchema[*models.User]().
//	    Field("name", "n", func(u *models.User) any { return u.Name }).
//	    FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })
//
//	// Fetch LIMIT+1 for accurate HasNextPage
//	fetchParams := cursor.BuildFetchParams(pageArgs, schema)
//	users, _ := fetcher.Fetch(ctx, fetchParams)
//
//	paginator, err := cursor.New(pageArgs, schema, users)
//	if err != nil {
//	    return nil, err  // Invalid sort field in pageArgs
//	}
//	conn, _ := cursor.BuildConnection(paginator, users, toDomainUser)
func New[T any](
	page PageArgs,
	schema *Schema[T],
	items []T,
	defaultLimit ...*int,
) (Paginator, error) {
	var defLimit *int
	if len(defaultLimit) > 0 {
		defLimit = defaultLimit[0]
	}

	params, err := parsePageArgs(page, schema, defLimit)
	if err != nil {
		return Paginator{}, err
	}

	// N+1 pattern: Check if we got more items than requested
	hasNextPage := len(items) > params.limit

	// Trim items to the requested limit
	trimmedItems := items
	if hasNextPage {
		trimmedItems = items[:params.limit]
	}

	pageInfo := newCursorBasedPageInfo(params.encoder, trimmedItems, params.limit, params.cursor, hasNextPage)

	return Paginator{
		limit:    params.limit,
		cursor:   params.cursor,
		PageInfo: pageInfo,
		orderBy:  params.orderBy,
		encoder:  params.encoder,
	}, nil
}

// BuildFetchParams creates FetchParams with automatic N+1 pattern for accurate HasNextPage detection.
// It uses the schema to validate PageArgs, get the encoder, and build the complete OrderBy clause.
func BuildFetchParams[T any](
	page PageArgs,
	schema *Schema[T],
) (paging.FetchParams, error) {
	params, err := parsePageArgs(page, schema, nil)
	if err != nil {
		return paging.FetchParams{}, err
	}

	return paging.FetchParams{
		Limit:   params.limit + 1, // N+1 for HasNextPage detection
		Cursor:  params.cursor,
		OrderBy: params.orderBy,
	}, nil
}

// GetPageInfo returns the PageInfo for this paginator.
// PageInfo contains functions to retrieve pagination metadata like
// cursors and whether next/previous pages exist.
//
// Note: TotalCount always returns nil for cursor pagination.
func (p *Paginator) GetPageInfo() paging.PageInfo {
	return p.PageInfo
}

// GetLimit returns the page size limit.
func (p *Paginator) GetLimit() int {
	return p.limit
}

// GetCursor returns the cursor position for this paginator.
// Returns nil if this is the first page.
func (p *Paginator) GetCursor() *paging.CursorPosition {
	return p.cursor
}

// GetOrderBy returns the OrderBy directives for this paginator.
func (p *Paginator) GetOrderBy() []paging.Sort {
	return p.orderBy
}

// BuildConnection creates a Relay-compliant GraphQL connection from a slice of items.
// It handles transformation from database models to domain models and automatically
// generates composite key cursors for each item.
//
// This function eliminates the manual boilerplate of building edges and nodes arrays,
// reducing repository code by 60-80%.
//
// The encoder is obtained from the paginator (which got it from the schema),
// ensuring encoder/OrderBy matching is enforced.
//
// Type parameters:
//   - From: Source type (e.g., *models.User from SQLBoiler)
//   - To: Target type (e.g., *domain.User for GraphQL)
//
// Parameters:
//   - paginator: The cursor paginator containing pagination state and encoder
//   - items: Slice of database records to transform
//   - transform: Function that converts database model to domain model
//
// Returns a Connection with edges, nodes, and pageInfo populated.
//
// Example usage:
//
//	// Before (manual boilerplate - 25+ lines):
//	result := &domain.UserConnection{PageInfo: &paginator.PageInfo}
//	for _, row := range dbUsers {
//	    user, err := toDomainUser(row)
//	    if err != nil { return nil, err }
//	    cursor, _ := encoder.Encode(row)
//	    result.Edges = append(result.Edges, domain.Edge{
//	        Cursor: *cursor,
//	        Node:   user,
//	    })
//	    result.Nodes = append(result.Nodes, user)
//	}
//
//	// After (using BuildConnection - 1 line):
//	return cursor.BuildConnection(paginator, dbUsers, toDomainUser)
func BuildConnection[From any, To any](
	paginator Paginator,
	items []From,
	transform func(From) (To, error),
) (*paging.Connection[To], error) {
	// Get encoder from paginator (type assert)
	encoder, ok := paginator.encoder.(paging.CursorEncoder[From])
	if !ok {
		return nil, fmt.Errorf("paginator encoder type mismatch")
	}

	// N+1 pattern: Trim items to the requested limit
	// If caller fetched LIMIT+1, we only want to return LIMIT items in the connection
	trimmedItems := items
	if len(items) > paginator.limit {
		trimmedItems = items[:paginator.limit]
	}

	return paging.BuildConnection(
		trimmedItems,
		paginator.PageInfo,
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

// newCursorBasedPageInfo creates PageInfo for cursor-based pagination.
// It uses the fetched items to generate cursors and determine page boundaries.
//
// Key differences from offset pagination:
//   - TotalCount always returns nil (cursor pagination doesn't need total count)
//   - HasNextPage uses N+1 pattern: passed in based on whether we got LIMIT+1 records
//   - StartCursor/EndCursor encode the first/last items' sort columns
//   - HasPreviousPage checks if cursor is not nil (has cursor = not first page)
func newCursorBasedPageInfo[T any](
	encoder paging.CursorEncoder[T],
	items []T,
	limit int,
	currentCursor *paging.CursorPosition,
	hasNextPage bool,
) paging.PageInfo {
	return paging.PageInfo{
		// TotalCount: Not available for cursor pagination
		TotalCount: func() (*int, error) {
			return nil, nil
		},

		// StartCursor: Encode first item (or nil if empty)
		StartCursor: func() (*string, error) {
			if len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[0])
		},

		// EndCursor: Encode last item (or nil if empty)
		EndCursor: func() (*string, error) {
			if len(items) == 0 {
				return nil, nil
			}
			return encoder.Encode(items[len(items)-1])
		},

		// HasNextPage: Determined by N+1 pattern (caller fetches LIMIT+1)
		// True if we got more items than the requested limit
		HasNextPage: func() (bool, error) {
			return hasNextPage, nil
		},

		// HasPreviousPage: True if we have a cursor (implies we're not on the first page)
		HasPreviousPage: func() (bool, error) {
			return currentCursor != nil, nil
		},
	}
}
