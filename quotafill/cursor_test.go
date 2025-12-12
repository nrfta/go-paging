package quotafill_test

import (
	"context"
	"testing"

	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/quotafill"
)

// Simple cursor encoder for testing
type simpleCursorEncoder struct{}

func (e *simpleCursorEncoder) Encode(item testItem) (*string, error) {
	if item.ID == 0 {
		return nil, nil
	}
	cursor := string(rune('A' + item.ID - 1))
	return &cursor, nil
}

func (e *simpleCursorEncoder) Decode(cursor string) (*paging.CursorPosition, error) {
	return nil, nil
}

func TestCursorGeneration(t *testing.T) {
	encoder := &simpleCursorEncoder{}

	basePaginator := newMockPaginator([][]testItem{
		{{ID: 1}, {ID: 2}, {ID: 3}},
	})

	wrapper := quotafill.Wrap(basePaginator, passAllFilter(), encoder)
	
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
	} else if *startCursor != "A" {
		t.Errorf("Expected StartCursor 'A', got %s", *startCursor)
	}
	
	endCursor, err := page.PageInfo.EndCursor()
	if err != nil {
		t.Fatalf("EndCursor failed: %v", err)
	}
	if endCursor == nil {
		t.Error("EndCursor should not be nil")
	} else if *endCursor != "B" {
		t.Errorf("Expected EndCursor 'B', got %s", *endCursor)
	}
}

func TestCursorNilForOffset(t *testing.T) {
	basePaginator := newMockPaginator([][]testItem{
		{{ID: 1}, {ID: 2}},
	})

	wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)
	
	first := 2
	args := &paging.PageArgs{First: &first}
	page, err := wrapper.Paginate(context.Background(), args)
	
	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}
	
	startCursor, _ := page.PageInfo.StartCursor()
	if startCursor != nil {
		t.Error("StartCursor should be nil when encoder is nil")
	}
	
	endCursor, _ := page.PageInfo.EndCursor()
	if endCursor != nil {
		t.Error("EndCursor should be nil when encoder is nil")
	}
}
