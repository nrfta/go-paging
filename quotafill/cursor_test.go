package quotafill_test

import (
	"context"
	"testing"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"
	"github.com/nrfta/paging-go/v2/quotafill"
)

func TestCursorGeneration(t *testing.T) {
	schema := testItemSchema()

	fetcher := newMockFetcher([]testItem{
		{ID: 1}, {ID: 2}, {ID: 3},
	})

	wrapper := quotafill.New[testItem](fetcher, passAllFilter(), schema)

	first := 2
	args := &paging.PageArgs{First: &first}
	page, err := wrapper.Paginate(context.Background(), args)

	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}

	if len(page.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(page.Nodes))
	}

	startCursor, err := page.PageInfo.StartCursor()
	if err != nil {
		t.Fatalf("StartCursor failed: %v", err)
	}
	if startCursor == nil {
		t.Error("StartCursor should not be nil")
	}

	endCursor, err := page.PageInfo.EndCursor()
	if err != nil {
		t.Fatalf("EndCursor failed: %v", err)
	}
	if endCursor == nil {
		t.Error("EndCursor should not be nil")
	}
}

func TestCursorNilForOffset(t *testing.T) {
	fetcher := newMockFetcher([]testItem{
		{ID: 1}, {ID: 2},
	})

	wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

	first := 2
	args := &paging.PageArgs{First: &first}
	page, err := wrapper.Paginate(context.Background(), args)

	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}

	startCursor, _ := page.PageInfo.StartCursor()
	if startCursor != nil {
		t.Error("StartCursor should be nil when schema is nil")
	}

	endCursor, _ := page.PageInfo.EndCursor()
	if endCursor != nil {
		t.Error("EndCursor should be nil when schema is nil")
	}
}

// Ensure cursor package is imported (used by testItemSchema in quotafill_test.go)
var _ = cursor.ASC
