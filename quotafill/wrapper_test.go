package quotafill_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/quotafill"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestQuotaFill(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "QuotaFill Suite")
}

// mockPaginator is a test double for paging.Paginator[T]
type mockPaginator[T any] struct {
	pages       [][]T           // Pages to return on successive calls
	currentPage int             // Current page index
	pageInfos   []paging.PageInfo // PageInfo for each page
}

func newMockPaginator[T any](pages [][]T) *mockPaginator[T] {
	pageInfos := make([]paging.PageInfo, len(pages))
	for i := range pages {
		hasNext := i < len(pages)-1
		pageInfos[i] = paging.PageInfo{
			HasNextPage: func(hn bool) func() (bool, error) {
				return func() (bool, error) { return hn, nil }
			}(hasNext),
			HasPreviousPage: func() (bool, error) { return i > 0, nil },
			StartCursor:     func() (*string, error) { return nil, nil },
			EndCursor: func(idx int) func() (*string, error) {
				return func() (*string, error) {
					if idx < len(pages)-1 {
						cursor := "cursor-page-" + string(rune('0'+idx))
						return &cursor, nil
					}
					return nil, nil
				}
			}(i),
			TotalCount: func() (*int, error) { return nil, nil },
		}
	}

	return &mockPaginator[T]{
		pages:       pages,
		currentPage: 0,
		pageInfos:   pageInfos,
	}
}

func (m *mockPaginator[T]) Paginate(ctx context.Context, args *paging.PageArgs) (*paging.Page[T], error) {
	if m.currentPage >= len(m.pages) {
		// No more pages
		return &paging.Page[T]{
			Nodes:    []T{},
			PageInfo: &paging.PageInfo{
				HasNextPage:     func() (bool, error) { return false, nil },
				HasPreviousPage: func() (bool, error) { return true, nil },
				StartCursor:     func() (*string, error) { return nil, nil },
				EndCursor:       func() (*string, error) { return nil, nil },
				TotalCount:      func() (*int, error) { return nil, nil },
			},
			Metadata: paging.Metadata{},
		}, nil
	}

	page := &paging.Page[T]{
		Nodes:    m.pages[m.currentPage],
		PageInfo: &m.pageInfos[m.currentPage],
		Metadata: paging.Metadata{},
	}

	m.currentPage++
	return page, nil
}

// Simple item type for testing
type testItem struct {
	ID     int
	Active bool
}

// mockCursorEncoder is a simple cursor encoder for testing
type mockCursorEncoder struct{}

func (e *mockCursorEncoder) Encode(item testItem) (*string, error) {
	cursor := "cursor-" + string(rune('0'+item.ID))
	return &cursor, nil
}

func (e *mockCursorEncoder) Decode(cursor string) (*paging.CursorPosition, error) {
	return nil, nil
}

var _ = Describe("QuotaFill Wrapper", func() {
	Describe("Basic Functionality", func() {
		It("should wrap a paginator successfully", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
			})

			wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)
			Expect(wrapper).ToNot(BeNil())
		})

		It("should return all items when filter passes everything", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
			})

			wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3))
			Expect(page.Metadata.Strategy).To(Equal("quotafill"))
			Expect(page.Metadata.ItemsExamined).To(Equal(5))
			Expect(page.Metadata.IterationsUsed).To(Equal(1))
		})

		It("should apply filter to remove items", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1, Active: true}, {ID: 2, Active: false}, {ID: 3, Active: true}},
			})

			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil)

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
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
			})

			wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)

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
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
			})

			wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)

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
			basePaginator := newMockPaginator([][]testItem{
				// Page 1: 4 items, 2 pass (need 2 more)
				{{ID: 1, Active: true}, {ID: 2, Active: false}, {ID: 3, Active: true}, {ID: 4, Active: false}},
				// Page 2: 4 items, 2 pass (quota filled: 4 total)
				{{ID: 5, Active: true}, {ID: 6, Active: false}, {ID: 7, Active: true}, {ID: 8, Active: false}},
			})

			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil)

			first := 3 // Request 3 items
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3), "Should return exactly 3 items")
			Expect(page.Metadata.IterationsUsed).To(Equal(2), "Should fetch 2 batches")
			Expect(page.Metadata.ItemsExamined).To(Equal(8), "Should examine 8 items total")

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue(), "Should have next page (got 4 filtered, wanted 3)")
		})

		It("should stop fetching when base paginator has no more data", func() {
			// Setup: Only 2 active items exist, but we request 5
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1, Active: true}, {ID: 2, Active: false}},
				{{ID: 3, Active: true}, {ID: 4, Active: false}},
				// No more pages
			})

			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil)

			first := 5 // Request 5 items, but only 2 pass the filter
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2), "Should return all 2 available items")
			Expect(page.Metadata.IterationsUsed).To(Equal(2), "Should fetch until no more data")

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse(), "Should have no next page")
		})
	})

	Describe("Adaptive Backoff", func() {
		It("should use backoff multipliers to optimize fetching", func() {
			// Setup: Sparse filter - only ID 9 is active
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1, Active: false}, {ID: 2, Active: false}, {ID: 3, Active: false}, {ID: 4, Active: false}},
				{{ID: 5, Active: false}, {ID: 6, Active: false}, {ID: 7, Active: false}, {ID: 8, Active: false}},
				{{ID: 9, Active: true}, {ID: 10, Active: false}, {ID: 11, Active: false}, {ID: 12, Active: false}},
			})

			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil)

			first := 1
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(1))
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">=", 2), "Should use multiple iterations with backoff")
		})

		It("should respect custom backoff multipliers", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1, Active: false}, {ID: 2, Active: false}},
				{{ID: 3, Active: true}},
			})

			// Use aggressive backoff [10, 20]
			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil,
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
			// Setup: Paginator that always has more data, but filter never passes anything
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}},
				{{ID: 2}},
				{{ID: 3}},
				{{ID: 4}},
				{{ID: 5}},
				{{ID: 6}},
			})

			wrapper := quotafill.Wrap(basePaginator, rejectAllFilter(), nil,
				quotafill.WithMaxIterations(3),
			)

			first := 10
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(0), "Should return empty results")
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("max_iterations"))
			Expect(page.Metadata.IterationsUsed).To(Equal(3))
		})

		It("should trigger max records examined safeguard", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
				{{ID: 4}, {ID: 5}, {ID: 6}},
				{{ID: 7}, {ID: 8}, {ID: 9}},
			})

			wrapper := quotafill.Wrap(basePaginator, rejectAllFilter(), nil,
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

			wrapper := quotafill.Wrap(slowPaginator, passAllFilter(), nil,
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

			wrapper := quotafill.Wrap(errorPaginator, passAllFilter(), nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			_, err := wrapper.Paginate(context.Background(), args)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database error"))
		})

		It("should propagate errors from filter function", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
			})

			filter := func(ctx context.Context, items []testItem) ([]testItem, error) {
				return nil, errors.New("authorization error")
			}

			wrapper := quotafill.Wrap(basePaginator, filter, nil)

			first := 3
			args := &paging.PageArgs{First: &first}
			_, err := wrapper.Paginate(context.Background(), args)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authorization error"))
		})
	})

	Describe("Metadata Tracking", func() {
		It("should track metadata correctly", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1, Active: true}, {ID: 2, Active: false}},
				{{ID: 3, Active: true}, {ID: 4, Active: false}},
			})

			wrapper := quotafill.Wrap(basePaginator, activeFilter(), nil)

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
		It("should generate cursors when encoder is provided", func() {
			encoder := &mockCursorEncoder{}

			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
			})

			wrapper := quotafill.Wrap(basePaginator, oddIDFilter(), encoder)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3), "Should return 3 filtered items (1, 3, 5)")

			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil(), "StartCursor should be generated")
			Expect(*startCursor).To(Equal("cursor-1"), "StartCursor should be for first item (ID=1)")

			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil(), "EndCursor should be generated")
			Expect(*endCursor).To(Equal("cursor-5"), "EndCursor should be for last item (ID=5)")
		})

		It("should return nil cursors when encoder is nil (offset pagination)", func() {
			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
			})

			wrapper := quotafill.Wrap(basePaginator, passAllFilter(), nil)

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
			encoder := &mockCursorEncoder{}

			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},          // First batch: 1, 3 pass
				{{ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}}, // Second batch: 5, 7 pass
			})

			wrapper := quotafill.Wrap(basePaginator, oddIDFilter(), encoder)

			first := 4
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(context.Background(), args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(4), "Should return 4 items (1, 3, 5, 7)")
			Expect(page.Metadata.IterationsUsed).To(Equal(2), "Should use 2 iterations")

			startCursor, _ := page.PageInfo.StartCursor()
			Expect(*startCursor).To(Equal("cursor-1"), "StartCursor should be for first item (ID=1)")

			endCursor, _ := page.PageInfo.EndCursor()
			Expect(*endCursor).To(Equal("cursor-7"), "EndCursor should be for last item (ID=7)")
		})

		It("should return nil cursors when no items after filtering", func() {
			encoder := &mockCursorEncoder{}

			basePaginator := newMockPaginator([][]testItem{
				{{ID: 1}, {ID: 2}, {ID: 3}},
			})

			wrapper := quotafill.Wrap(basePaginator, rejectAllFilter(), encoder)

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

func (s *slowMockPaginator) Paginate(ctx context.Context, args *paging.PageArgs) (*paging.Page[testItem], error) {
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

	return &paging.Page[testItem]{
		Nodes: s.items,
		PageInfo: &paging.PageInfo{
			HasNextPage:     func() (bool, error) { return false, nil },
			HasPreviousPage: func() (bool, error) { return false, nil },
			StartCursor:     func() (*string, error) { return nil, nil },
			EndCursor:       func() (*string, error) { return nil, nil },
			TotalCount:      func() (*int, error) { return nil, nil },
		},
		Metadata: paging.Metadata{},
	}, nil
}

// errorMockPaginator simulates errors for error handling testing
type errorMockPaginator struct {
	err error
}

func (e *errorMockPaginator) Paginate(ctx context.Context, args *paging.PageArgs) (*paging.Page[testItem], error) {
	return nil, e.err
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
