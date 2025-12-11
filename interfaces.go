package paging

import "context"

// Paginator is the core interface for all pagination strategies.
// Implementations include offset-based, cursor-based, and quota-fill pagination.
//
// Type parameter T is the item type being paginated (e.g., User, Post, Organization).
//
// Example implementations:
//   - offset.Paginator: Traditional offset/limit pagination
//   - cursor.Paginator: High-performance cursor-based pagination (Phase 2)
//   - quotafill.Wrapper: Filtering-aware pagination (Phase 3)
type Paginator[T any] interface {
	// Paginate executes pagination and returns a page of results.
	// The PageArgs contain the page size (First) and cursor position (After).
	Paginate(ctx context.Context, args *PageArgs) (*Page[T], error)
}

// Page represents a single page of paginated results.
// It contains the actual items, pagination metadata, and observability information.
//
// Type parameter T is the item type being paginated.
type Page[T any] struct {
	// Nodes contains the items for this page.
	Nodes []T

	// PageInfo contains pagination metadata (hasNextPage, cursors, etc.)
	PageInfo *PageInfo

	// Metadata provides observability and debugging information.
	// Useful for monitoring pagination performance and behavior.
	Metadata Metadata
}

// Metadata provides observability and debugging information about pagination execution.
// This data is useful for monitoring, alerting, and optimization.
type Metadata struct {
	// Strategy identifies which pagination strategy was used.
	// Values: "offset", "cursor", "quotafill"
	Strategy string

	// QueryTimeMs is the total time spent executing database queries.
	QueryTimeMs int64

	// ItemsExamined is the total number of items fetched from the database.
	// For quota-fill, this may be higher than the returned item count due to filtering.
	ItemsExamined int

	// IterationsUsed is the number of fetch iterations performed.
	// For simple pagination this is 1. For quota-fill it may be higher.
	IterationsUsed int

	// SafeguardHit indicates if a safeguard was triggered during quota-fill.
	// Values: nil (no safeguard), "max_iterations", "max_records", "timeout"
	SafeguardHit *string
}

// Fetcher abstracts database queries for any ORM or database layer.
// This interface allows paginators to work with SQLBoiler, GORM, sqlc,
// or raw SQL without being tightly coupled to any specific ORM.
//
// Type parameter T is the database model type (e.g., *models.User from SQLBoiler).
//
// Example implementation:
//
//	type sqlboilerFetcher struct {
//	    queryFunc func(...qm.QueryMod) ([]*models.User, error)
//	    countFunc func(...qm.QueryMod) (int64, error)
//	}
type Fetcher[T any] interface {
	// Fetch retrieves items from storage based on the given parameters.
	// It should apply limit, offset/cursor, ordering, and any custom filters.
	Fetch(ctx context.Context, params FetchParams) ([]T, error)

	// Count returns the total number of items matching the filters (without pagination).
	// This is optional for some pagination strategies (cursor-based doesn't need it).
	// Return 0 if count is not supported or too expensive to compute.
	Count(ctx context.Context, params FetchParams) (int64, error)
}

// FetchParams contains all parameters needed to fetch a page of data.
// Paginators construct these parameters based on their strategy.
type FetchParams struct {
	// Limit is the maximum number of items to fetch.
	Limit int

	// Offset is the number of items to skip (for offset-based pagination).
	Offset int

	// Cursor is the position marker (for cursor/keyset pagination).
	// Contains the values of sort columns for the last item on the previous page.
	Cursor *CursorPosition

	// Filters contains custom filter criteria.
	// These are strategy-agnostic and passed through to the Fetcher implementation.
	Filters map[string]any

	// OrderBy specifies the sort order for results.
	OrderBy []OrderBy
}

// OrderBy represents a sort directive for query results.
type OrderBy struct {
	// Column is the name of the column to sort by.
	Column string

	// Desc indicates descending order. False means ascending.
	Desc bool
}

// CursorPosition encodes the pagination position for cursor-based strategies.
// It contains the values of the sort columns for resuming pagination.
//
// Example for sorting by (created_at DESC, id ASC):
//
//	CursorPosition{
//	    Values: map[string]any{
//	        "created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
//	        "id": "uuid-123",
//	    },
//	}
//
// This translates to: WHERE (created_at, id) < ('2024-01-01', 'uuid-123')
type CursorPosition struct {
	// Values maps column names to their values at the cursor position.
	Values map[string]any
}

// FilterFunc is a generic filter function for quota-fill pagination.
// It receives a batch of items and returns a filtered subset.
//
// This function is intentionally generic - it doesn't need to know what
// kind of filtering is being applied. Common use cases:
//   - Authorization: Filter items the user is allowed to see
//   - Soft-deletes: Filter out deleted items
//   - Status filtering: Only return items with status="active"
//   - Feature flags: Filter based on enabled features
//
// Type parameter T is the item type being filtered.
//
// Example authorization filter:
//
//	filterFunc := func(ctx context.Context, orgs []*models.Organization) ([]*models.Organization, error) {
//	    checks := make([]AuthCheck, len(orgs))
//	    for i, org := range orgs {
//	        checks[i] = AuthCheck{UserID: userID, OrgID: org.ID, Action: "read"}
//	    }
//	    results, err := authzClient.BatchCheck(ctx, checks)
//	    if err != nil {
//	        return nil, err
//	    }
//	    authorized := []*models.Organization{}
//	    for _, org := range orgs {
//	        if results[org.ID] {
//	            authorized = append(authorized, org)
//	        }
//	    }
//	    return authorized, nil
//	}
type FilterFunc[T any] func(ctx context.Context, items []T) ([]T, error)

// CursorEncoder handles cursor serialization for cursor-based pagination.
// It converts items into opaque cursor strings and decodes cursor strings
// back into CursorPosition values.
//
// Type parameter T is the item type (e.g., *models.User).
//
// Example implementation:
//
//	type compositeCursorEncoder struct {
//	    extractor func(*models.User) map[string]any
//	}
//
//	func (e *compositeCursorEncoder) Encode(user *models.User) (*string, error) {
//	    values := e.extractor(user) // {"created_at": ..., "id": ...}
//	    data, _ := json.Marshal(values)
//	    encoded := base64.StdEncoding.EncodeToString(data)
//	    return &encoded, nil
//	}
type CursorEncoder[T any] interface {
	// Encode creates an opaque cursor string from an item.
	// The cursor should encode the values needed to resume pagination
	// (typically the sort key values).
	Encode(item T) (*string, error)

	// Decode extracts cursor position from an opaque cursor string.
	// Returns nil if the cursor is empty or invalid.
	Decode(cursor string) (*CursorPosition, error)
}
