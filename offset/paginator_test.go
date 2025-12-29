package offset_test

import (
	"context"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/offset"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testUser struct {
	ID   int
	Name string
}

// mockFetcher creates a simple in-memory fetcher for testing
func mockFetcher(totalCount int64, allItems []*testUser) paging.Fetcher[*testUser] {
	return &testFetcher{
		totalCount: totalCount,
		allItems:   allItems,
	}
}

type testFetcher struct {
	totalCount int64
	allItems   []*testUser
}

func (f *testFetcher) Fetch(ctx context.Context, params paging.FetchParams) ([]*testUser, error) {
	start := params.Offset
	end := start + params.Limit
	if start >= len(f.allItems) {
		return []*testUser{}, nil
	}
	if end > len(f.allItems) {
		end = len(f.allItems)
	}
	return f.allItems[start:end], nil
}

func (f *testFetcher) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return f.totalCount, nil
}

// generateTestUsers creates a slice of test users
func generateTestUsers(count int) []*testUser {
	users := make([]*testUser, count)
	for i := 0; i < count; i++ {
		users[i] = &testUser{ID: i + 1, Name: "User"}
	}
	return users
}

var _ = Describe("Paginator", func() {
	var (
		ctx    context.Context
		fetcher paging.Fetcher[*testUser]
		paginator paging.Paginator[*testUser]
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create 100 test users
		allUsers := generateTestUsers(100)
		fetcher = mockFetcher(100, allUsers)
		paginator = offset.New(fetcher)
	})

	Describe("Basic functionality", func() {
		It("uses the default limit when no pageArgs.First is provided", func() {
			args := &paging.PageArgs{}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			// Should return 50 items (default page size)
			Expect(page.Nodes).To(HaveLen(50))

			totalCount, _ := page.PageInfo.TotalCount()
			Expect(*totalCount).To(Equal(100))
		})

		It("parses the pageArgs correctly", func() {
			first := 10
			args := &paging.PageArgs{
				First: &first,
				After: offset.EncodeCursor(20),
			}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			// Should return 10 items starting at offset 20
			Expect(page.Nodes).To(HaveLen(10))
			Expect(page.Nodes[0].ID).To(Equal(21)) // offset 20 = ID 21 (1-indexed)
		})

		It("creates a page info with correct pagination metadata", func() {
			first := 10
			args := &paging.PageArgs{
				First: &first,
				After: offset.EncodeCursor(20),
			}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())

			totalCount, _ := page.PageInfo.TotalCount()
			Expect(*totalCount).To(Equal(100))

			hasNextPage, _ := page.PageInfo.HasNextPage()
			Expect(hasNextPage).To(Equal(true))

			hasPreviousPage, _ := page.PageInfo.HasPreviousPage()
			Expect(hasPreviousPage).To(Equal(true))

			// StartCursor should point to first item on current page (offset 20)
			// Cursor encoding is offset + 1, so cursor = 21
			startCursor, _ := page.PageInfo.StartCursor()
			Expect(startCursor).To(Equal(offset.EncodeCursor(21)))

			// EndCursor should point to last item on current page (offset 29)
			// Cursor encoding is offset + 1, so cursor = 30
			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).To(Equal(offset.EncodeCursor(30)))
		})
	})

	Describe("PaginateOption", func() {
		It("should use WithDefaultSize when First is nil", func() {
			args := &paging.PageArgs{}

			page, err := paginator.Paginate(ctx, args, paging.WithDefaultSize(25))
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(25))
		})

		It("should cap page size with WithMaxSize", func() {
			first := 500
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
			Expect(err).ToNot(HaveOccurred())
			// Capped to 100, but only 100 total items exist
			Expect(page.Nodes).To(HaveLen(100))
		})

		It("should allow page size within MaxSize", func() {
			first := 50
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args, paging.WithMaxSize(100))
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(50))
		})

		It("should cap large requests to DefaultMaxPageSize by default", func() {
			first := 5000
			args := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, args)
			Expect(err).ToNot(HaveOccurred())
			// Capped to DefaultMaxPageSize (1000), but only 100 items exist
			Expect(page.Nodes).To(HaveLen(100))
		})
	})
})
