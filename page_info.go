package paging

// PageInfo contains metadata about a paginated result set.
// It uses function fields to enable lazy evaluation of pagination metadata,
// which is useful when some information (like total count) may be expensive to compute.
//
// All functions return both a value and an error to support async computation
// or database queries that may fail.
type PageInfo struct {
	TotalCount      func() (*int, error)
	HasPreviousPage func() (bool, error)
	HasNextPage     func() (bool, error)
	StartCursor     func() (*string, error)
	EndCursor       func() (*string, error)
}

// NewEmptyPageInfo returns an empty instance of PageInfo.
// This is useful when you need to satisfy PageInfo requirements but don't have
// pagination data yet (e.g., empty result sets, error cases, or placeholder responses).
//
// All functions return nil or false:
//   - TotalCount: nil
//   - StartCursor: nil
//   - EndCursor: nil
//   - HasNextPage: false
//   - HasPreviousPage: false
//
// Example usage:
//
//	if len(items) == 0 {
//	    return &Connection{
//	        Nodes:    []Item{},
//	        Edges:    []Edge[Item]{},
//	        PageInfo: NewEmptyPageInfo(),
//	    }, nil
//	}
func NewEmptyPageInfo() PageInfo {
	return PageInfo{
		TotalCount:      func() (*int, error) { return nil, nil },
		StartCursor:     func() (*string, error) { return nil, nil },
		EndCursor:       func() (*string, error) { return nil, nil },
		HasNextPage:     func() (bool, error) { return false, nil },
		HasPreviousPage: func() (bool, error) { return false, nil },
	}
}
