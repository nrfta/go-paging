package cursor_test

import (
	"errors"
	"time"

	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/cursor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Paginator", func() {
	var (
		encoder *cursor.CompositeCursorEncoder[*testUser]
		users   []*testUser
	)

	BeforeEach(func() {
		// Create test users
		users = []*testUser{
			{ID: "user-1", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Age: 25},
			{ID: "user-2", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Age: 30},
			{ID: "user-3", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Age: 35},
		}

		// Create encoder
		encoder = cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
			return map[string]any{
				"created_at": u.CreatedAt,
				"id":         u.ID,
			}
		}).(*cursor.CompositeCursorEncoder[*testUser])
	})

	Describe("Basic functionality", func() {
		It("uses the default limit when no pageArgs.First is provided", func() {
			page := &paging.PageArgs{}

			paginator := cursor.New(page, encoder, users)

			Expect(paginator.GetLimit()).To(Equal(50))
			Expect(paginator.GetCursor()).To(BeNil())
		})

		It("parses the pageArgs correctly", func() {
			// Encode a cursor
			cursorStr, _ := encoder.Encode(users[0])

			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: cursorStr,
			}

			paginator := cursor.New(page, encoder, users)

			Expect(paginator.GetLimit()).To(Equal(10))
			Expect(paginator.GetCursor()).ToNot(BeNil())
			Expect(paginator.GetCursor().Values).To(HaveKey("id"))
			Expect(paginator.GetCursor().Values).To(HaveKey("created_at"))
		})

		It("handles nil cursor gracefully", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			Expect(paginator.GetLimit()).To(Equal(10))
			Expect(paginator.GetCursor()).To(BeNil())
		})

		It("handles zero limit with default", func() {
			first := 0
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			Expect(paginator.GetLimit()).To(Equal(50)) // Falls back to default
		})

		It("uses custom default limit when provided", func() {
			customDefault := 25
			page := &paging.PageArgs{}

			paginator := cursor.New(page, encoder, users, &customDefault)

			Expect(paginator.GetLimit()).To(Equal(25))
		})
	})

	Describe("PageInfo", func() {
		It("creates a page info with correct metadata", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			// TotalCount should return nil for cursor pagination
			totalCount, err := paginator.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(BeNil())

			// HasNextPage should be false (only 3 items, limit is 10)
			hasNextPage, err := paginator.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNextPage).To(BeFalse())

			// HasPreviousPage should be false (no cursor)
			hasPreviousPage, err := paginator.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPreviousPage).To(BeFalse())

			// StartCursor should encode first item
			startCursor, err := paginator.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil())

			// EndCursor should encode last item
			endCursor, err := paginator.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil())
		})

		It("indicates HasNextPage when more items exist (N+1 pattern)", func() {
			first := 2 // Request 2 items
			page := &paging.PageArgs{
				First: &first,
			}

			// N+1 pattern: Pass 3 items (LIMIT+1) to signal there's a next page
			paginator := cursor.New(page, encoder, users)

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeTrue()) // len(items) > limit means HasNextPage = true
		})

		It("indicates no HasNextPage when exactly limit items exist", func() {
			first := 3 // Request 3 items, we have exactly 3
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse()) // len(items) == limit means HasNextPage = false
		})

		It("indicates HasPreviousPage when cursor is provided", func() {
			cursorStr, _ := encoder.Encode(users[1])

			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: cursorStr,
			}

			paginator := cursor.New(page, encoder, users)

			hasPreviousPage, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPreviousPage).To(BeTrue()) // Has cursor = not first page
		})

		It("handles empty results", func() {
			emptyUsers := []*testUser{}
			page := &paging.PageArgs{}

			paginator := cursor.New(page, encoder, emptyUsers)

			startCursor, _ := paginator.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil())

			endCursor, _ := paginator.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil())

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse())
		})
	})

	Describe("OrderBy", func() {
		It("should use default created_at DESC when no sort columns provided", func() {
			page := &paging.PageArgs{}

			paginator := cursor.New(page, encoder, users)

			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(1))
			Expect(orderBy[0].Column).To(Equal("created_at"))
			Expect(orderBy[0].Desc).To(BeTrue())
		})

		It("should set DESC flag when specified", func() {
			page := paging.WithSortBy(&paging.PageArgs{}, true, "created_at", "id")

			paginator := cursor.New(page, encoder, users)

			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(2))
			Expect(orderBy[0].Column).To(Equal("created_at"))
			Expect(orderBy[0].Desc).To(BeTrue())
			Expect(orderBy[1].Column).To(Equal("id"))
			Expect(orderBy[1].Desc).To(BeTrue())
		})

		It("should set ASC when DESC is false", func() {
			page := paging.WithSortBy(&paging.PageArgs{}, false, "name", "id")

			paginator := cursor.New(page, encoder, users)

			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(2))
			Expect(orderBy[0].Column).To(Equal("name"))
			Expect(orderBy[0].Desc).To(BeFalse())
			Expect(orderBy[1].Column).To(Equal("id"))
			Expect(orderBy[1].Desc).To(BeFalse())
		})

		It("should handle single column", func() {
			page := paging.WithSortBy(&paging.PageArgs{}, true, "email")

			paginator := cursor.New(page, encoder, users)

			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(1))
			Expect(orderBy[0].Column).To(Equal("email"))
			Expect(orderBy[0].Desc).To(BeTrue())
		})
	})

	Describe("BuildConnection", func() {
		It("should build a connection with edges and nodes", func() {
			first := 3
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			// Transform function (just return same user for testing)
			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(paginator, users, encoder, transform)

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

		It("should handle transform errors", func() {
			first := 3
			page := &paging.PageArgs{
				First: &first,
			}

			paginator := cursor.New(page, encoder, users)

			// Transform function that returns error
			transform := func(u *testUser) (*testUser, error) {
				return nil, errors.New("transform error")
			}

			conn, err := cursor.BuildConnection(paginator, users, encoder, transform)

			Expect(err).To(HaveOccurred())
			Expect(conn).To(BeNil())
		})

		It("should handle empty results", func() {
			emptyUsers := []*testUser{}
			page := &paging.PageArgs{}

			paginator := cursor.New(page, encoder, emptyUsers)

			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(paginator, emptyUsers, encoder, transform)

			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(0))
			Expect(conn.Edges).To(HaveLen(0))
		})
	})
})
