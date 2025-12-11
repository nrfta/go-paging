package paging

// PageArgs represents pagination query parameters.
// It follows the Relay cursor pagination specification with First (page size)
// and After (cursor) fields, plus optional sorting configuration.
type PageArgs struct {
	First      *int    `json:"first,omitempty"`
	After      *string `json:"after,omitempty"`
	sortByCols []string
	isDesc     bool
}

// WithSortBy configures the sort columns and direction for pagination.
// It modifies the PageArgs and returns it for method chaining.
// If pa is nil, a new PageArgs is created.
//
// Example:
//
//	args := WithSortBy(nil, true, "created_at", "id")
//	// Results in ORDER BY created_at, id DESC
func WithSortBy(pa *PageArgs, isDesc bool, cols ...string) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.isDesc = isDesc
	pa.sortByCols = cols
	return pa
}

// GetFirst returns the requested page size.
func (pa *PageArgs) GetFirst() *int {
	return pa.First
}

// GetAfter returns the cursor position for pagination.
func (pa *PageArgs) GetAfter() *string {
	return pa.After
}

// SortByCols returns the list of columns to sort by.
func (pa *PageArgs) SortByCols() []string {
	return pa.sortByCols
}

// IsDesc returns whether sorting should be in descending order.
func (pa *PageArgs) IsDesc() bool {
	return pa.isDesc
}

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
