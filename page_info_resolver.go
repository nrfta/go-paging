package paging

import (
	"context"
)

// PageInfoResolver interface
type PageInfoResolver interface {
	HasPreviousPage(ctx context.Context, pageInfo *PageInfo) (bool, error)
	HasNextPage(ctx context.Context, pageInfo *PageInfo) (bool, error)
	TotalCount(ctx context.Context, pageInfo *PageInfo) (*int, error)
	StartCursor(ctx context.Context, pageInfo *PageInfo) (*string, error)
	EndCursor(ctx context.Context, pageInfo *PageInfo) (*string, error)
}

type pageInfoResolver struct{}

// NewPageInfoResolver returns the resolver for PageInfo
func NewPageInfoResolver() PageInfoResolver {
	return &pageInfoResolver{}
}

func (r *pageInfoResolver) TotalCount(ctx context.Context, pageInfo *PageInfo) (*int, error) {
	return pageInfo.TotalCount()
}

func (r *pageInfoResolver) HasPreviousPage(ctx context.Context, pageInfo *PageInfo) (bool, error) {
	return pageInfo.HasPreviousPage()
}

func (r *pageInfoResolver) HasNextPage(ctx context.Context, pageInfo *PageInfo) (bool, error) {
	return pageInfo.HasNextPage()
}

func (r *pageInfoResolver) StartCursor(ctx context.Context, pageInfo *PageInfo) (*string, error) {
	return pageInfo.StartCursor()
}

func (r *pageInfoResolver) EndCursor(ctx context.Context, pageInfo *PageInfo) (*string, error) {
	return pageInfo.EndCursor()
}
