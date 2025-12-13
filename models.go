package paging

// PageArgs represents pagination query parameters.
// It follows the Relay cursor pagination specification with First (page size),
// After (cursor), and SortBy (sort configuration) fields.
type PageArgs struct {
	First  *int    `json:"first,omitempty"`
	After  *string `json:"after,omitempty"`
	SortBy []Sort  `json:"sortBy,omitempty"`
}

// WithSortBy configures a single sort column and direction for pagination.
// It modifies the PageArgs and returns it for method chaining.
// If pa is nil, a new PageArgs is created.
//
// Example:
//
//	args := WithSortBy(nil, "created_at", true)
//	// Results in ORDER BY created_at DESC
func WithSortBy(pa *PageArgs, column string, desc bool) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.SortBy = []Sort{{Column: column, Desc: desc}}
	return pa
}

// WithMultiSort configures multiple sort columns with individual directions.
// It modifies the PageArgs and returns it for method chaining.
// If pa is nil, a new PageArgs is created.
//
// Example:
//
//	args := WithMultiSort(nil,
//	    Sort{Column: "created_at", Desc: true},
//	    Sort{Column: "name", Desc: false},
//	)
//	// Results in ORDER BY created_at DESC, name ASC
func WithMultiSort(pa *PageArgs, sorts ...Sort) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.SortBy = sorts
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

// GetSortBy returns the list of sort specifications.
func (pa *PageArgs) GetSortBy() []Sort {
	return pa.SortBy
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
