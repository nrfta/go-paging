package paging

// PageArgs is used as the query inputs
type PageArgs struct {
	First      *int    `json:"first,omitempty"`
	After      *string `json:"after,omitempty"`
	sortByCols []string
	isDesc     bool
}

func WithSortBy(pa *PageArgs, isDesc bool, cols ...string) *PageArgs {
	if pa == nil {
		pa = &PageArgs{}
	}

	pa.isDesc = isDesc
	pa.sortByCols = cols
	return pa
}

// PageInfo is the base struct for building PageInfo. It expects inline functions for all the fields
// We use inline functions so that one can build a lazy page info
type PageInfo struct {
	TotalCount      func() (*int, error)
	HasPreviousPage func() (bool, error)
	HasNextPage     func() (bool, error)
	StartCursor     func() (*string, error)
	EndCursor       func() (*string, error)
}
