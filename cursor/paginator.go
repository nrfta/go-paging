package cursor

import (
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

// Paginator is the paginator for cursor-based pagination.
// It encapsulates limit, cursor position, and page metadata for keyset queries.
//
// Unlike offset pagination, cursor pagination:
//   - Does not require totalCount
//   - Uses cursor position instead of offset
//   - Provides O(1) performance regardless of page depth
//   - Requires composite indexes on sort columns
type Paginator struct {
	limit    int
	cursor   *paging.CursorPosition
	PageInfo paging.PageInfo
	orderBy  []paging.OrderBy
}

// New creates a new cursor paginator.
//
// Parameters:
//   - page: Pagination arguments including page size, cursor, and sorting
//   - encoder: Cursor encoder for decoding After cursor and encoding result cursors
//   - items: The items fetched from the database (should be LIMIT+1 for accurate HasNextPage)
//   - defaultLimit: Optional default page size (defaults to 50 if not provided)
//
// The paginator automatically handles:
//   - N+1 pattern: Detects if you fetched LIMIT+1 for accurate HasNextPage detection
//   - Item trimming: Trims results to requested limit if N+1 was fetched
//   - Default page size of 50 records
//   - Zero-value protection to prevent divide-by-zero errors
//   - Cursor decoding using the provided encoder
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
//	// Fetch LIMIT+1 for accurate HasNextPage
//	fetchParams := paging.FetchParams{Limit: 10 + 1, ...}
//	users, _ := fetcher.Fetch(ctx, fetchParams)
//
//	encoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
//	    return map[string]any{"created_at": u.CreatedAt, "id": u.ID}
//	})
//
//	paginator := cursor.New(pageArgs, encoder, users)
//	conn, _ := cursor.BuildConnection(paginator, users, encoder, toDomainUser)
func New[T any](
	page PageArgs,
	encoder paging.CursorEncoder[T],
	items []T,
	defaultLimit ...*int,
) Paginator {
	// Determine limit (same as offset pagination)
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

	// Decode cursor from After field
	var cursor *paging.CursorPosition
	if page != nil && page.GetAfter() != nil {
		cursor, _ = encoder.Decode(*page.GetAfter())
	}

	// Build orderBy from page args
	orderBy := buildOrderBy(page)

	// N+1 pattern: Check if we got more items than requested
	// If caller fetched LIMIT+1 and we got LIMIT+1, there's a next page
	hasNextPage := len(items) > limit

	// Trim items to the requested limit
	trimmedItems := items
	if hasNextPage {
		trimmedItems = items[:limit]
	}

	// Build PageInfo with trimmed items and accurate hasNextPage
	pageInfo := newCursorBasedPageInfo(encoder, trimmedItems, limit, cursor, hasNextPage)

	return Paginator{
		limit:    limit,
		cursor:   cursor,
		PageInfo: pageInfo,
		orderBy:  orderBy,
	}
}

// BuildFetchParams creates FetchParams with automatic N+1 pattern for accurate HasNextPage detection.
func BuildFetchParams[T any](
	page PageArgs,
	encoder paging.CursorEncoder[T],
	orderBy []paging.OrderBy,
) paging.FetchParams {
	limit := defaultLimitVal
	if page != nil && page.GetFirst() != nil && *page.GetFirst() > 0 {
		limit = *page.GetFirst()
	}

	if limit == 0 {
		limit = defaultLimitVal
	}

	var cursor *paging.CursorPosition
	if page != nil && page.GetAfter() != nil && encoder != nil {
		cursor, _ = encoder.Decode(*page.GetAfter())
	}

	if len(orderBy) == 0 {
		orderBy = buildOrderBy(page)
	}

	return paging.FetchParams{
		Limit:   limit + 1,
		Cursor:  cursor,
		OrderBy: orderBy,
	}
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
func (p *Paginator) GetOrderBy() []paging.OrderBy {
	return p.orderBy
}

// BuildConnection creates a Relay-compliant GraphQL connection from a slice of items.
// It handles transformation from database models to domain models and automatically
// generates composite key cursors for each item.
//
// This function eliminates the manual boilerplate of building edges and nodes arrays,
// reducing repository code by 60-80%.
//
// Type parameters:
//   - From: Source type (e.g., *models.User from SQLBoiler)
//   - To: Target type (e.g., *domain.User for GraphQL)
//
// Parameters:
//   - paginator: The cursor paginator containing pagination state
//   - items: Slice of database records to transform
//   - encoder: Cursor encoder for generating cursors from items
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
//	return cursor.BuildConnection(paginator, dbUsers, encoder, toDomainUser)
func BuildConnection[From any, To any](
	paginator Paginator,
	items []From,
	encoder paging.CursorEncoder[From],
	transform func(From) (To, error),
) (*paging.Connection[To], error) {
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

// defaultOrderBy is the fallback when no sort columns are specified.
var defaultOrderBy = []paging.OrderBy{{Column: "created_at", Desc: true}}

// buildOrderBy constructs OrderBy directives from PageArgs.
// Defaults to created_at DESC if no sort columns are specified.
func buildOrderBy(page PageArgs) []paging.OrderBy {
	if page == nil {
		return defaultOrderBy
	}

	cols := page.SortByCols()
	if len(cols) == 0 {
		return defaultOrderBy
	}

	isDesc := page.IsDesc()
	orderBy := make([]paging.OrderBy, len(cols))
	for i, col := range cols {
		orderBy[i] = paging.OrderBy{
			Column: col,
			Desc:   isDesc,
		}
	}

	return orderBy
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
