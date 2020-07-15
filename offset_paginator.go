package paging

import (
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const defaultLimitVal = 50

// OffsetPaginator is the paginator for offset based pagination
type OffsetPaginator struct {
	Limit    int
	Offset   int
	PageInfo PageInfo
}

// NewOffsetPaginator creates a new offset paginator
func NewOffsetPaginator(
	page *PageArgs,
	totalCount int64,
	defaultLimit ...*int,
) OffsetPaginator {
	if page == nil {
		page = &PageArgs{}
	}

	limit := defaultLimitVal
	if len(defaultLimit) > 0 && defaultLimit[0] != nil {
		limit = *defaultLimit[0]
	}

	if page.First != nil {
		limit = *page.First
	}

	offset := DecodeOffsetCursor(page.After)

	return OffsetPaginator{
		Limit:    limit,
		Offset:   offset,
		PageInfo: NewOffsetBasedPageInfo(&limit, totalCount, offset),
	}
}

// QueryMods returns the sqlboilder query mods with pagination concerns
func (p *OffsetPaginator) QueryMods() []qm.QueryMod {
	return []qm.QueryMod{
		qm.Offset(p.Offset),
		qm.Limit(p.Limit),
	}
}
