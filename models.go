package paging

// PageArgs is used as the query inputs
type PageArgs struct {
	First *int
	After *string
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
