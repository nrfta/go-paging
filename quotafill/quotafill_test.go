package quotafill_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"
	"github.com/nrfta/paging-go/v2/quotafill"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestQuotaFill(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "QuotaFill Suite")
}

// mockFetcher is a test double for paging.Fetcher[T]
// It maintains internal state for sequential fetching (like a real DB query would)
type mockFetcher[T any] struct {
	allItems []T // All items available
	offset   int // Current offset for sequential fetching
}

func newMockFetcher[T any](items []T) *mockFetcher[T] {
	return &mockFetcher[T]{
		allItems: items,
		offset:   0,
	}
}

func (m *mockFetcher[T]) Fetch(ctx context.Context, params paging.FetchParams) ([]T, error) {
	// Use cursor position if provided (for cursor-based pagination)
	start := m.offset
	if params.Cursor != nil {
		// Extract offset from cursor
		if offsetVal, ok := params.Cursor.Values["offset"].(int); ok {
			start = offsetVal
		} else if offsetVal, ok := params.Cursor.Values["offset"].(float64); ok {
			start = int(offsetVal)
		}
	}

	end := start + params.Limit

	if start >= len(m.allItems) {
		// No more items
		return []T{}, nil
	}

	if end > len(m.allItems) {
		end = len(m.allItems)
	}

	items := m.allItems[start:end]

	// Update offset for next sequential fetch (when no cursor provided)
	m.offset = end

	return items, nil
}

func (m *mockFetcher[T]) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return int64(len(m.allItems)), nil
}

// Simple item type for testing
type testItem struct {
	ID     int
	Active bool
}

// testItemSchema creates a schema for cursor-based pagination tests
func testItemSchema() *cursor.Schema[testItem] {
	return cursor.NewSchema[testItem]().
		FixedField("id", cursor.DESC, "i", func(item testItem) any { return item.ID })
}

var _ = Describe("QuotaFill Wrapper", func() {
	Describe("Basic Functionality", func() {
		It("should wrap a fetcher successfully", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
			})

			wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)
			Expect(wrapper).ToNot(BeNil())
		})

		It("should return all items when filter passes everything", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
			})

			wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3))
			Expect(page.Metadata.Strategy).To(Equal("quotafill"))
			Expect(page.Metadata.ItemsExamined).To(Equal(4)) // Trimmed from N+1 (fetched 5, trimmed to 4, got 4 after filter)
			Expect(page.Metadata.IterationsUsed).To(Equal(1))
		})

		It("should apply filter to remove items", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1, Active: true}, {ID: 2, Active: false}, {ID: 3, Active: true},
			})

			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil)

			first := 2
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2))
			Expect(page.Nodes[0].ID).To(Equal(1))
			Expect(page.Nodes[1].ID).To(Equal(3))
		})
	})

	Describe("N+1 Pattern for HasNextPage", func() {
		It("should detect HasNextPage=true when fetching exactly limit+1 items", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
			})

			wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

			first := 3 // Request 3 items, will fetch 4 (N+1) internally
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3), "Should return exactly 3 items (trimmed from 4)")

			hasNext, err := page.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue(), "Should detect next page exists (got 4, wanted 3)")
		})

		It("should detect HasNextPage=false when fetching exactly limit items", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
			})

			wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

			first := 3 // Request 3 items, try to fetch 4 (N+1) but only get 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3))

			hasNext, err := page.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeFalse(), "Should detect no next page (got exactly 3)")
		})
	})

	Describe("Iterative Fetching", func() {
		It("should fetch multiple batches until quota is filled", func() {
			// Setup: 50% pass rate filter (Active alternates: true, false, true, false...)
			fetcher := newMockFetcher([]testItem{
				// Iteration 1: Fetch 5 (N+1), trim to 4, examine 4, get 2 filtered (IDs 1,3)
				{ID: 1, Active: true}, {ID: 2, Active: false}, {ID: 3, Active: true}, {ID: 4, Active: false},
				// Iteration 2: Offset advanced to 5, fetch remaining 3 items
				{ID: 5, Active: true}, {ID: 6, Active: false}, {ID: 7, Active: true},
			})

			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil)

			first := 3 // Request 3 items
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3), "Should return exactly 3 items")
			Expect(page.Metadata.IterationsUsed).To(Equal(2), "Should fetch 2 batches")
			Expect(page.Metadata.ItemsExamined).To(Equal(6), "Should examine 6 items total (4 + 2)")

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse(), "Should not have next page (only got 3 filtered items)")
		})

		It("should stop fetching when base paginator has no more data", func() {
			// Setup: Only 2 active items exist, but we request 5
			fetcher := newMockFetcher([]testItem{
				{ID: 1, Active: true}, {ID: 2, Active: false},
				{ID: 3, Active: true}, {ID: 4, Active: false},
				// No more pages
			})

			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil)

			first := 5 // Request 5 items, but only 2 pass the filter
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2), "Should return all 2 available items")
			Expect(page.Metadata.IterationsUsed).To(Equal(1), "Detects no more data in first iteration")

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse(), "Should have no next page")
		})
	})

	Describe("Adaptive Backoff", func() {
		It("should use backoff multipliers to optimize fetching", func() {
			// Setup: Sparse filter - only ID 9 is active
			fetcher := newMockFetcher([]testItem{
				{ID: 1, Active: false}, {ID: 2, Active: false}, {ID: 3, Active: false}, {ID: 4, Active: false},
				{ID: 5, Active: false}, {ID: 6, Active: false}, {ID: 7, Active: false}, {ID: 8, Active: false},
				{ID: 9, Active: true}, {ID: 10, Active: false}, {ID: 11, Active: false}, {ID: 12, Active: false},
			})

			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil)

			first := 1
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(1))
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">=", 2), "Should use multiple iterations with backoff")
		})

		It("should respect custom backoff multipliers", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1, Active: false}, {ID: 2, Active: false},
				{ID: 3, Active: true},
			})

			// Use aggressive backoff [10, 20]
			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil,
				quotafill.WithBackoffMultipliers([]int{10, 20}),
			)

			first := 1
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(1))
		})
	})

	Describe("Safeguards", func() {
		It("should trigger max iterations safeguard", func() {
			// Setup: Fetcher with lots of data, but filter never passes anything
			// Need enough items so each iteration can fetch and still have more
			items := make([]testItem, 100)
			for i := range items {
				items[i] = testItem{ID: i + 1}
			}
			fetcher := newMockFetcher(items)

			wrapper := quotafill.New[testItem](fetcher, rejectAllFilter(), nil,
				quotafill.WithMaxIterations(3),
			)

			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(0), "Should return empty results")
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("max_iterations"))
			Expect(page.Metadata.IterationsUsed).To(Equal(3))
		})

		It("should trigger max records examined safeguard", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
				{ID: 4}, {ID: 5}, {ID: 6},
				{ID: 7}, {ID: 8}, {ID: 9},
			})

			wrapper := quotafill.New[testItem](fetcher, rejectAllFilter(), nil,
				quotafill.WithMaxRecordsExamined(5),
			)

			first := 10
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(0))
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("max_records"))
			Expect(page.Metadata.ItemsExamined).To(BeNumerically("<=", 5))
		})

		It("should trigger timeout safeguard", func() {
			// Create a paginator that sleeps to trigger timeout
			slowPaginator := &slowMockPaginator{
				delay: 500 * time.Millisecond,
				items: []testItem{{ID: 1}, {ID: 2}, {ID: 3}},
			}

			wrapper := quotafill.New[testItem](slowPaginator, passAllFilter(), nil,
				quotafill.WithTimeout(100*time.Millisecond),
			)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("timeout"))
		})
	})

	Describe("Error Handling", func() {
		It("should propagate errors from base paginator", func() {
			errorPaginator := &errorMockPaginator{
				err: errors.New("database error"),
			}

			wrapper := quotafill.New[testItem](errorPaginator, passAllFilter(), nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			_, err := wrapper.Paginate(context.Background(), args)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database error"))
		})

		It("should propagate errors from filter function", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
			})

			filter := func(ctx context.Context, items []testItem) ([]testItem, error) {
				return nil, errors.New("authorization error")
			}

			wrapper := quotafill.New[testItem](fetcher, filter, nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			_, err := wrapper.Paginate(context.Background(), args)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authorization error"))
		})
	})

	Describe("Metadata Tracking", func() {
		It("should track metadata correctly", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1, Active: true}, {ID: 2, Active: false},
				{ID: 3, Active: true}, {ID: 4, Active: false},
			})

			wrapper := quotafill.New[testItem](fetcher, activeFilter(), nil)

			first := 2
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())

			// Verify metadata
			Expect(page.Metadata.Strategy).To(Equal("quotafill"))
			Expect(page.Metadata.ItemsExamined).To(BeNumerically(">", 0))
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">", 0))
			Expect(page.Metadata.QueryTimeMs).To(BeNumerically(">=", 0))
		})
	})

	Describe("Cursor Handling", func() {
		It("should generate cursors when schema is provided", func() {
			schema := testItemSchema()

			fetcher := newMockFetcher([]testItem{
				// Need extra odd IDs to account for N+1 offset advancement
				{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}, {ID: 9},
			})

			wrapper := quotafill.New[testItem](fetcher, oddIDFilter(), schema)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3), "Should return 3 filtered items")

			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil(), "StartCursor should be generated")

			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil(), "EndCursor should be generated")
		})

		It("should return nil cursors when schema is nil (offset pagination)", func() {
			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
			})

			wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

			first := 2
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())

			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).To(BeNil(), "StartCursor should be nil for offset pagination")

			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).To(BeNil(), "EndCursor should be nil for offset pagination")
		})

		It("should generate cursors correctly after multiple iterations", func() {
			schema := testItemSchema()

			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},          // First fetch: 1, 3 pass
				{ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}, // Second fetch: 5, 7 pass
			})

			wrapper := quotafill.New[testItem](fetcher, oddIDFilter(), schema)

			first := 4
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(4), "Should return 4 items (1, 3, 5, 7)")
			Expect(page.Metadata.IterationsUsed).To(Equal(2), "Should use 2 iterations")

			startCursor, _ := page.PageInfo.StartCursor()
			Expect(startCursor).ToNot(BeNil(), "StartCursor should be generated")

			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).ToNot(BeNil(), "EndCursor should be generated")
		})

		It("should return nil cursors when no items after filtering", func() {
			schema := testItemSchema()

			fetcher := newMockFetcher([]testItem{
				{ID: 1}, {ID: 2}, {ID: 3},
			})

			wrapper := quotafill.New[testItem](fetcher, rejectAllFilter(), schema)

			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(0))

			startCursor, _ := page.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil(), "StartCursor should be nil when no items")

			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil(), "EndCursor should be nil when no items")
		})
	})
})

// slowMockPaginator simulates a slow paginator for timeout testing
type slowMockPaginator struct {
	delay time.Duration
	items []testItem
}

func (s *slowMockPaginator) Fetch(ctx context.Context, params paging.FetchParams) ([]testItem, error) {
	// Check if context is already cancelled before sleeping
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Sleep to trigger timeout, but respect context cancellation
	timer := time.NewTimer(s.delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Sleep completed
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return s.items, nil
}

func (s *slowMockPaginator) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return int64(len(s.items)), nil
}

// errorMockPaginator simulates errors for error handling testing
type errorMockPaginator struct {
	err error
}

func (e *errorMockPaginator) Fetch(ctx context.Context, params paging.FetchParams) ([]testItem, error) {
	return nil, e.err
}

func (e *errorMockPaginator) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return 0, e.err
}

// activeFilter returns a filter that passes only items with Active=true
func activeFilter() func(context.Context, []testItem) ([]testItem, error) {
	return func(ctx context.Context, items []testItem) ([]testItem, error) {
		filtered := []testItem{}
		for _, item := range items {
			if item.Active {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}
}

// oddIDFilter returns a filter that passes only items with odd IDs
func oddIDFilter() func(context.Context, []testItem) ([]testItem, error) {
	return func(ctx context.Context, items []testItem) ([]testItem, error) {
		filtered := []testItem{}
		for _, item := range items {
			if item.ID%2 == 1 {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}
}

// passAllFilter returns a filter that passes all items unchanged
func passAllFilter() func(context.Context, []testItem) ([]testItem, error) {
	return func(ctx context.Context, items []testItem) ([]testItem, error) {
		return items, nil
	}
}

// rejectAllFilter returns a filter that rejects all items
func rejectAllFilter() func(context.Context, []testItem) ([]testItem, error) {
	return func(ctx context.Context, items []testItem) ([]testItem, error) {
		return []testItem{}, nil
	}
}

var _ = Describe("PaginateOption", func() {
	It("should use WithDefaultSize when First is nil", func() {
		fetcher := newMockFetcher([]testItem{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
			{ID: 6}, {ID: 7}, {ID: 8}, {ID: 9}, {ID: 10},
			{ID: 11}, {ID: 12}, {ID: 13}, {ID: 14}, {ID: 15},
			{ID: 16}, {ID: 17}, {ID: 18}, {ID: 19}, {ID: 20},
			{ID: 21}, {ID: 22}, {ID: 23}, {ID: 24}, {ID: 25},
			{ID: 26},
		})

		wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

		args := &paging.PageArgs{}
		page, err := wrapper.Paginate(context.Background(), args,
			paging.WithDefaultSize(25),
		)

		Expect(err).ToNot(HaveOccurred())
		Expect(page.Nodes).To(HaveLen(25))
	})

	It("should cap page size with WithMaxSize", func() {
		fetcher := newMockFetcher([]testItem{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		})

		wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

		first := 100
		args := &paging.PageArgs{First: &first}
		page, err := wrapper.Paginate(context.Background(), args,
			paging.WithMaxSize(3),
		)

		Expect(err).ToNot(HaveOccurred())
		Expect(page.Nodes).To(HaveLen(3))
	})

	It("should allow page size within MaxSize", func() {
		fetcher := newMockFetcher([]testItem{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		})

		wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

		first := 3
		args := &paging.PageArgs{First: &first}
		page, err := wrapper.Paginate(context.Background(), args,
			paging.WithMaxSize(100),
		)

		Expect(err).ToNot(HaveOccurred())
		Expect(page.Nodes).To(HaveLen(3))
	})

	It("should not enforce max when no options are provided", func() {
		// Generate 60 items (more than default max of 50)
		items := make([]testItem, 60)
		for i := range items {
			items[i] = testItem{ID: i + 1}
		}
		fetcher := newMockFetcher(items)

		wrapper := quotafill.New[testItem](fetcher, passAllFilter(), nil)

		first := 55
		args := &paging.PageArgs{First: &first}
		page, err := wrapper.Paginate(context.Background(), args)

		Expect(err).ToNot(HaveOccurred())
		Expect(page.Nodes).To(HaveLen(55))
	})
})
