package cursor_test

import (
	"context"
	"time"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockFetcher creates a simple in-memory fetcher for testing cursor pagination
func mockFetcher(allItems []*testUser) paging.Fetcher[*testUser] {
	return &testFetcher{allItems: allItems}
}

type testFetcher struct {
	allItems []*testUser
}

func (f *testFetcher) Fetch(ctx context.Context, params paging.FetchParams) ([]*testUser, error) {
	// Simple in-memory filtering based on cursor
	var result []*testUser
	startIdx := 0

	// If cursor exists, find where to start
	if params.Cursor != nil {
		if idVal, ok := params.Cursor.Values["id"]; ok {
			if id, ok := idVal.(string); ok {
				for i, u := range f.allItems {
					if u.ID == id {
						startIdx = i + 1 // Start after cursor position
						break
					}
				}
			}
		}
	}

	// Collect items
	for i := startIdx; i < len(f.allItems) && len(result) < params.Limit; i++ {
		result = append(result, f.allItems[i])
	}

	return result, nil
}

func (f *testFetcher) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return int64(len(f.allItems)), nil
}

var _ = Describe("Paginator", func() {
	var (
		ctx      context.Context
		schema   *cursor.Schema[*testUser]
		users    []*testUser
		fetcher  paging.Fetcher[*testUser]
		paginator paging.Paginator[*testUser]
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create test users
		users = []*testUser{
			{ID: "user-1", Name: "Alice", Email: "alice@example.com", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Age: 25},
			{ID: "user-2", Name: "Bob", Email: "bob@example.com", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Age: 30},
			{ID: "user-3", Name: "Charlie", Email: "charlie@example.com", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Age: 35},
			{ID: "user-4", Name: "Diana", Email: "diana@example.com", CreatedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC), Age: 40},
			{ID: "user-5", Name: "Eve", Email: "eve@example.com", CreatedAt: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC), Age: 45},
		}

		// Create schema with all sortable fields
		schema = cursor.NewSchema[*testUser]().
			Field("created_at", "c", func(u *testUser) any { return u.CreatedAt }).
			Field("name", "n", func(u *testUser) any { return u.Name }).
			Field("email", "e", func(u *testUser) any { return u.Email }).
			FixedField("id", cursor.DESC, "i", func(u *testUser) any { return u.ID })

		fetcher = mockFetcher(users)
		paginator = cursor.New(fetcher, schema)
	})

	Describe("Basic functionality", func() {
		It("uses the default limit when no pageArgs.First is provided", func() {
			args := &paging.PageArgs{}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			// Should return all 5 users (less than default 50)
			Expect(page.Nodes).To(HaveLen(5))
			Expect(page.Metadata.Strategy).To(Equal("cursor"))
		})

		It("parses the pageArgs correctly with cursor", func() {
			// First, get a cursor from the first page
			first := 2
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2))

			// Get the end cursor
			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).ToNot(BeNil())

			// Use it for next page
			nextArgs := &paging.PageArgs{
				First: &first,
				After: endCursor,
			}

			nextPage, err := paginator.Paginate(ctx, nextArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(nextPage.Nodes).To(HaveLen(2))
			// Should start after user-2
			Expect(nextPage.Nodes[0].ID).To(Equal("user-3"))
		})

		It("handles nil cursor gracefully", func() {
			first := 3
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			Expect(page.Nodes).To(HaveLen(3))
			Expect(page.Nodes[0].ID).To(Equal("user-1"))
		})
	})

	Describe("PageInfo", func() {
		It("creates a page info with correct metadata", func() {
			first := 2
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			// TotalCount should return nil for cursor pagination
			totalCount, err := page.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(BeNil())

			// HasNextPage should be true (5 users, limit 2)
			hasNextPage, err := page.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNextPage).To(BeTrue())

			// HasPreviousPage should be false (no cursor)
			hasPreviousPage, err := page.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPreviousPage).To(BeFalse())

			// StartCursor should encode first item
			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil())

			// EndCursor should encode last item
			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil())
		})

		It("indicates no HasNextPage when all items fetched", func() {
			first := 10 // More than we have
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			hasNextPage, _ := page.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse())
		})

		It("indicates HasPreviousPage when cursor is provided", func() {
			// Get cursor from first page
			first := 2
			args := &paging.PageArgs{First: &first}
			firstPage, _ := paginator.Paginate(ctx, args)
			endCursor, _ := firstPage.PageInfo.EndCursor()

			// Second page should have HasPreviousPage = true
			nextArgs := &paging.PageArgs{
				First: &first,
				After: endCursor,
			}

			page, err := paginator.Paginate(ctx, nextArgs)
			Expect(err).ToNot(HaveOccurred())

			hasPreviousPage, _ := page.PageInfo.HasPreviousPage()
			Expect(hasPreviousPage).To(BeTrue())
		})

		It("handles empty results", func() {
			emptyFetcher := mockFetcher([]*testUser{})
			emptyPaginator := cursor.New(emptyFetcher, schema)
			args := &paging.PageArgs{}

			page, err := emptyPaginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			startCursor, _ := page.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil())

			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil())

			hasNextPage, _ := page.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse())
		})
	})

	Describe("PaginateOption", func() {
		It("should use WithDefaultSize when First is nil", func() {
			args := &paging.PageArgs{}

			page, err := paginator.Paginate(ctx, args, paging.WithDefaultSize(3))
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3))
		})

		It("should cap page size with WithMaxSize", func() {
			first := 10
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args, paging.WithMaxSize(2))
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2))
		})

		It("should allow page size within MaxSize", func() {
			first := 3
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args, paging.WithMaxSize(5))
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(3))
		})
	})

	Describe("BuildConnection", func() {
		It("should build a connection with edges and nodes", func() {
			first := 3
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			// Transform function (just return same user for testing)
			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(page, schema, args, transform)

			Expect(err).ToNot(HaveOccurred())
			Expect(conn).ToNot(BeNil())
			Expect(conn.Nodes).To(HaveLen(3))
			Expect(conn.Edges).To(HaveLen(3))

			// Verify first edge has cursor
			Expect(conn.Edges[0].Cursor).ToNot(BeEmpty())
			Expect(conn.Edges[0].Node).To(Equal(users[0]))

			// Verify pageInfo is attached
			Expect(conn.PageInfo).ToNot(BeZero())
		})

		It("should handle empty results", func() {
			emptyFetcher := mockFetcher([]*testUser{})
			emptyPaginator := cursor.New(emptyFetcher, schema)
			args := &paging.PageArgs{}

			page, err := emptyPaginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(page, schema, args, transform)

			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(0))
			Expect(conn.Edges).To(HaveLen(0))
		})
	})
})
