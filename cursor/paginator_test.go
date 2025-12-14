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
		schema *cursor.Schema[*testUser]
		users  []*testUser
	)

	BeforeEach(func() {
		// Create test users
		users = []*testUser{
			{ID: "user-1", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Age: 25},
			{ID: "user-2", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Age: 30},
			{ID: "user-3", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Age: 35},
		}

		// Create schema with all sortable fields
		schema = cursor.NewSchema[*testUser]().
			Field("created_at", "c", func(u *testUser) any { return u.CreatedAt }).
			Field("name", "n", func(u *testUser) any { return u.Name }).
			Field("email", "e", func(u *testUser) any { return u.Email }).
			FixedField("id", cursor.DESC, "i", func(u *testUser) any { return u.ID })
	})

	Describe("Basic functionality", func() {
		It("uses the default limit when no pageArgs.First is provided", func() {
			page := &paging.PageArgs{}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			Expect(paginator.GetLimit()).To(Equal(50))
			Expect(paginator.GetCursor()).To(BeNil())
		})

		It("parses the pageArgs correctly", func() {
			// Create PageArgs with sort field to ensure it's in cursor
			pageArgsForCursor := paging.WithSortBy(&paging.PageArgs{}, "created_at", true)

			// Get encoder and encode a cursor
			encoder, _ := schema.EncoderFor(pageArgsForCursor)
			cursorStr, _ := encoder.Encode(users[0])

			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: cursorStr,
				SortBy: []paging.Sort{{Column: "created_at", Desc: true}},
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

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

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			Expect(paginator.GetLimit()).To(Equal(10))
			Expect(paginator.GetCursor()).To(BeNil())
		})

		It("handles zero limit with default", func() {
			first := 0
			page := &paging.PageArgs{
				First: &first,
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			Expect(paginator.GetLimit()).To(Equal(50)) // Falls back to default
		})

		It("uses custom default limit when provided", func() {
			customDefault := 25
			page := &paging.PageArgs{}

			paginator, err := cursor.New(page, schema, users, &customDefault)
			Expect(err).ToNot(HaveOccurred())

			Expect(paginator.GetLimit()).To(Equal(25))
		})
	})

	Describe("PageInfo", func() {
		It("creates a page info with correct metadata", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

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
			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeTrue()) // len(items) > limit means HasNextPage = true
		})

		It("indicates no HasNextPage when exactly limit items exist", func() {
			first := 3 // Request 3 items, we have exactly 3
			page := &paging.PageArgs{
				First: &first,
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse()) // len(items) == limit means HasNextPage = false
		})

		It("indicates HasPreviousPage when cursor is provided", func() {
			encoder, _ := schema.EncoderFor(&paging.PageArgs{})
			cursorStr, _ := encoder.Encode(users[1])

			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: cursorStr,
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			hasPreviousPage, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPreviousPage).To(BeTrue()) // Has cursor = not first page
		})

		It("handles empty results", func() {
			emptyUsers := []*testUser{}
			page := &paging.PageArgs{}

			paginator, err := cursor.New(page, schema, emptyUsers)
			Expect(err).ToNot(HaveOccurred())

			startCursor, _ := paginator.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil())

			endCursor, _ := paginator.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil())

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(BeFalse())
		})
	})

	Describe("OrderBy", func() {
		It("should include fixed field when no sort columns provided", func() {
			page := &paging.PageArgs{}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			// Schema automatically includes fixed "id" field
			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(1))
			Expect(orderBy[0].Column).To(Equal("id"))
			Expect(orderBy[0].Desc).To(BeTrue()) // Fixed field is DESC
		})

		It("should include user sorts and fixed field", func() {
			page := paging.WithSortBy(&paging.PageArgs{}, "created_at", true)

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			// Schema includes user sort + fixed "id" field
			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(2))
			Expect(orderBy[0].Column).To(Equal("created_at"))
			Expect(orderBy[0].Desc).To(BeTrue())
			Expect(orderBy[1].Column).To(Equal("id"))
			Expect(orderBy[1].Desc).To(BeTrue()) // Fixed field
		})

		It("should support multiple user-sortable columns", func() {
			page := paging.WithMultiSort(&paging.PageArgs{},
				paging.Sort{Column: "name", Desc: false},
				paging.Sort{Column: "email", Desc: true},
			)

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			// Schema includes user sorts + fixed "id" field
			orderBy := paginator.GetOrderBy()
			Expect(orderBy).To(HaveLen(3))
			Expect(orderBy[0].Column).To(Equal("name"))
			Expect(orderBy[0].Desc).To(BeFalse())
			Expect(orderBy[1].Column).To(Equal("email"))
			Expect(orderBy[1].Desc).To(BeTrue())
			Expect(orderBy[2].Column).To(Equal("id"))
			Expect(orderBy[2].Desc).To(BeTrue()) // Fixed field
		})

		It("should validate sort fields and return error for invalid fields", func() {
			page := paging.WithSortBy(&paging.PageArgs{}, "invalid_field", true)

			_, err := cursor.New(page, schema, users)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sort field: invalid_field"))
		})
	})

	Describe("BuildConnection", func() {
		It("should build a connection with edges and nodes", func() {
			first := 3
			page := &paging.PageArgs{
				First: &first,
			}

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			// Transform function (just return same user for testing)
			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(paginator, users, transform)

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

			paginator, err := cursor.New(page, schema, users)
			Expect(err).ToNot(HaveOccurred())

			// Transform function that returns error
			transform := func(u *testUser) (*testUser, error) {
				return nil, errors.New("transform error")
			}

			conn, err := cursor.BuildConnection(paginator, users, transform)

			Expect(err).To(HaveOccurred())
			Expect(conn).To(BeNil())
		})

		It("should handle empty results", func() {
			emptyUsers := []*testUser{}
			page := &paging.PageArgs{}

			paginator, err := cursor.New(page, schema, emptyUsers)
			Expect(err).ToNot(HaveOccurred())

			transform := func(u *testUser) (*testUser, error) {
				return u, nil
			}

			conn, err := cursor.BuildConnection(paginator, emptyUsers, transform)

			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(0))
			Expect(conn.Edges).To(HaveLen(0))
		})
	})
})
