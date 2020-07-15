package paging

import (
	"math"
)

// NewOffsetBasedPageInfo returns a new PageInfo object with data filled in, based on offset pagination
func NewOffsetBasedPageInfo(
	pageSize *int,
	totalCount int64,
	currentOffset int,
) PageInfo {
	count := int(totalCount)
	endOffset := count - int(math.Mod(float64(count), float64(*pageSize)))

	if endOffset == count {
		endOffset = count - *pageSize
	}

	return PageInfo{
		TotalCount:      func() (*int, error) { return &count, nil },
		StartCursor:     func() (*string, error) { return EncodeOffsetCursor(0), nil },
		EndCursor:       func() (*string, error) { return EncodeOffsetCursor(endOffset), nil },
		HasNextPage:     func() (bool, error) { return (currentOffset+*pageSize < count), nil },
		HasPreviousPage: func() (bool, error) { return (currentOffset-*pageSize > 0), nil },
	}
}

// NewEmptyPageInfo returns a empty instance of PageInfo. Useful for when working on a new page to be able to fullfil PageInfo requirements
func NewEmptyPageInfo() *PageInfo {
	return &PageInfo{
		TotalCount:      func() (*int, error) { return nil, nil },
		StartCursor:     func() (*string, error) { return nil, nil },
		EndCursor:       func() (*string, error) { return nil, nil },
		HasNextPage:     func() (bool, error) { return false, nil },
		HasPreviousPage: func() (bool, error) { return false, nil },
	}
}
