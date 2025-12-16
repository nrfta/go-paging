package paging_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/offset"
)

// Mock database models
type DBUser struct {
	ID        int
	Name      string
	Email     string
	CreatedAt string
}

// Mock domain models
type DomainUser struct {
	ID        string
	FullName  string
	EmailAddr string
}

// mockOffsetFetcher creates a simple in-memory fetcher for testing
func mockOffsetFetcher(allUsers []DBUser, totalCount int64) paging.Fetcher[DBUser] {
	return &offsetTestFetcher{
		allUsers:   allUsers,
		totalCount: totalCount,
	}
}

type offsetTestFetcher struct {
	allUsers   []DBUser
	totalCount int64
}

func (f *offsetTestFetcher) Fetch(ctx context.Context, params paging.FetchParams) ([]DBUser, error) {
	start := params.Offset
	end := start + params.Limit
	if start >= len(f.allUsers) {
		return []DBUser{}, nil
	}
	if end > len(f.allUsers) {
		end = len(f.allUsers)
	}
	return f.allUsers[start:end], nil
}

func (f *offsetTestFetcher) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return f.totalCount, nil
}

var _ = Describe("Connection and Edge", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("BuildConnection", func() {
		It("should build a connection with edges and nodes", func() {
			// Setup: Create mock database records
			dbUsers := []DBUser{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
				{ID: 2, Name: "Bob", Email: "bob@example.com"},
				{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
			}

			// Setup: Create mock PageInfo
			pageInfo := paging.PageInfo{
				HasNextPage:     func() (bool, error) { return true, nil },
				HasPreviousPage: func() (bool, error) { return false, nil },
				StartCursor:     func() (*string, error) { c := "cursor:0"; return &c, nil },
				EndCursor:       func() (*string, error) { c := "cursor:3"; return &c, nil },
				TotalCount:      func() (*int, error) { count := 100; return &count, nil },
			}

			// Setup: Define transform function
			transform := func(db DBUser) (*DomainUser, error) {
				return &DomainUser{
					ID:        fmt.Sprintf("user-%d", db.ID),
					FullName:  db.Name,
					EmailAddr: db.Email,
				}, nil
			}

			// Setup: Define cursor encoder
			cursorEncoder := func(i int, db DBUser) string {
				return fmt.Sprintf("cursor:%d", db.ID)
			}

			// Execute: Build connection
			conn, err := paging.BuildConnection(dbUsers, pageInfo, cursorEncoder, transform)

			// Assert: No error
			Expect(err).ToNot(HaveOccurred())
			Expect(conn).ToNot(BeNil())

			// Assert: Nodes are correctly transformed
			Expect(conn.Nodes).To(HaveLen(3))
			Expect(conn.Nodes[0].ID).To(Equal("user-1"))
			Expect(conn.Nodes[0].FullName).To(Equal("Alice"))
			Expect(conn.Nodes[1].ID).To(Equal("user-2"))
			Expect(conn.Nodes[2].ID).To(Equal("user-3"))

			// Assert: Edges are correctly built
			Expect(conn.Edges).To(HaveLen(3))
			Expect(conn.Edges[0].Cursor).To(Equal("cursor:1"))
			Expect(conn.Edges[0].Node).To(Equal(conn.Nodes[0]))
			Expect(conn.Edges[1].Cursor).To(Equal("cursor:2"))
			Expect(conn.Edges[2].Cursor).To(Equal("cursor:3"))

			// Assert: PageInfo is attached and functional
			hasNext, _ := conn.PageInfo.HasNextPage()
			hasPrev, _ := conn.PageInfo.HasPreviousPage()
			totalCount, _ := conn.PageInfo.TotalCount()
			Expect(hasNext).To(BeTrue())
			Expect(hasPrev).To(BeFalse())
			Expect(*totalCount).To(Equal(100))
		})

		It("should handle empty result set", func() {
			dbUsers := []DBUser{}
			pageInfo := paging.PageInfo{
				HasNextPage:     func() (bool, error) { return false, nil },
				HasPreviousPage: func() (bool, error) { return false, nil },
				TotalCount:      func() (*int, error) { count := 0; return &count, nil },
			}

			transform := func(db DBUser) (*DomainUser, error) {
				return &DomainUser{}, nil
			}

			cursorEncoder := func(i int, db DBUser) string {
				return fmt.Sprintf("cursor:%d", db.ID)
			}

			conn, err := paging.BuildConnection(dbUsers, pageInfo, cursorEncoder, transform)

			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(BeEmpty())
			Expect(conn.Edges).To(BeEmpty())
		})

		It("should propagate transform errors", func() {
			dbUsers := []DBUser{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
				{ID: 2, Name: "Bob", Email: "invalid"},
			}

			pageInfo := paging.PageInfo{}

			// Transform that fails on invalid email
			transform := func(db DBUser) (*DomainUser, error) {
				if db.Email == "invalid" {
					return nil, fmt.Errorf("invalid email: %s", db.Email)
				}
				return &DomainUser{
					ID:        fmt.Sprintf("user-%d", db.ID),
					FullName:  db.Name,
					EmailAddr: db.Email,
				}, nil
			}

			cursorEncoder := func(i int, db DBUser) string {
				return fmt.Sprintf("cursor:%d", db.ID)
			}

			conn, err := paging.BuildConnection(dbUsers, pageInfo, cursorEncoder, transform)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transform item at index 1"))
			Expect(err.Error()).To(ContainSubstring("invalid email"))
			Expect(conn).To(BeNil())
		})
	})

	Describe("offset.BuildConnection", func() {
		It("should build connection with offset-based cursors", func() {
			// Setup: Create mock data
			allUsers := []DBUser{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
				{ID: 2, Name: "Bob", Email: "bob@example.com"},
				{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
				{ID: 4, Name: "Diana", Email: "diana@example.com"},
				{ID: 5, Name: "Eve", Email: "eve@example.com"},
			}

			// Create paginator with fetcher
			fetcher := mockOffsetFetcher(allUsers, 10)
			paginator := offset.New(fetcher)

			// Paginate first page (2 items)
			first := 2
			pageArgs := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2))

			// Setup: Transform function
			transform := func(db DBUser) (*DomainUser, error) {
				return &DomainUser{
					ID:        fmt.Sprintf("user-%d", db.ID),
					FullName:  db.Name,
					EmailAddr: db.Email,
				}, nil
			}

			// Execute: Build connection using offset helper
			conn, err := offset.BuildConnection(page, transform)

			// Assert: No error
			Expect(err).ToNot(HaveOccurred())
			Expect(conn).ToNot(BeNil())

			// Assert: Nodes are transformed
			Expect(conn.Nodes).To(HaveLen(2))
			Expect(conn.Nodes[0].ID).To(Equal("user-1"))
			Expect(conn.Nodes[1].ID).To(Equal("user-2"))

			// Assert: Edges have sequential cursors
			Expect(conn.Edges).To(HaveLen(2))
			cursor1 := offset.DecodeCursor(&conn.Edges[0].Cursor)
			cursor2 := offset.DecodeCursor(&conn.Edges[1].Cursor)
			Expect(cursor1).To(Equal(1))
			Expect(cursor2).To(Equal(2))

			// Assert: PageInfo metadata
			hasNext, _ := conn.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())
			totalCountPtr, _ := conn.PageInfo.TotalCount()
			Expect(*totalCountPtr).To(Equal(10))
		})

		It("should handle second page with offset", func() {
			// Setup: Create mock data
			allUsers := []DBUser{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
				{ID: 2, Name: "Bob", Email: "bob@example.com"},
				{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
				{ID: 4, Name: "Diana", Email: "diana@example.com"},
				{ID: 5, Name: "Eve", Email: "eve@example.com"},
			}

			// Create paginator
			fetcher := mockOffsetFetcher(allUsers, 10)
			paginator := offset.New(fetcher)

			// Paginate second page (starting at offset 2)
			first := 2
			cursor := offset.EncodeCursor(2)
			pageArgs := &paging.PageArgs{
				First: &first,
				After: cursor,
			}

			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(2))
			Expect(page.Nodes[0].ID).To(Equal(3)) // 3rd user (offset 2)
			Expect(page.Nodes[1].ID).To(Equal(4)) // 4th user (offset 3)

			transform := func(db DBUser) (*DomainUser, error) {
				return &DomainUser{
					ID:       fmt.Sprintf("user-%d", db.ID),
					FullName: db.Name,
				}, nil
			}

			conn, err := offset.BuildConnection(page, transform)

			Expect(err).ToNot(HaveOccurred())

			// Assert: Cursors account for offset
			cursor1 := offset.DecodeCursor(&conn.Edges[0].Cursor)
			cursor2 := offset.DecodeCursor(&conn.Edges[1].Cursor)
			Expect(cursor1).To(Equal(3)) // offset 2 + index 0 + 1
			Expect(cursor2).To(Equal(4)) // offset 2 + index 1 + 1
		})
	})

	Describe("Real-world use case", func() {
		It("should eliminate repository boilerplate", func() {
			// Setup: Create mock data
			allUsers := []DBUser{
				{ID: 1, Name: "Alice", Email: "alice@example.com"},
				{ID: 2, Name: "Bob", Email: "bob@example.com"},
				{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
				{ID: 4, Name: "Diana", Email: "diana@example.com"},
				{ID: 5, Name: "Eve", Email: "eve@example.com"},
			}

			fetcher := mockOffsetFetcher(allUsers, 10)
			paginator := offset.New(fetcher)

			first := 3
			pageArgs := &paging.PageArgs{First: &first}

			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// AFTER: Using BuildConnection (new API)
			// This is now the ONLY way to build a connection
			conn, err := offset.BuildConnection(page, func(db DBUser) (*DomainUser, error) {
				return &DomainUser{
					ID:        fmt.Sprintf("user-%d", db.ID),
					FullName:  db.Name,
					EmailAddr: db.Email,
				}, nil
			})

			// Assert: Succeeds
			Expect(err).ToNot(HaveOccurred())

			// Assert: Results are correct
			Expect(conn.Nodes).To(HaveLen(3))
			Expect(conn.Edges).To(HaveLen(3))

			Expect(conn.Nodes[0].ID).To(Equal("user-1"))
			Expect(conn.Nodes[1].ID).To(Equal("user-2"))
			Expect(conn.Nodes[2].ID).To(Equal("user-3"))

			// Verify cursors
			cursor1 := offset.DecodeCursor(&conn.Edges[0].Cursor)
			cursor2 := offset.DecodeCursor(&conn.Edges[1].Cursor)
			cursor3 := offset.DecodeCursor(&conn.Edges[2].Cursor)
			Expect(cursor1).To(Equal(1))
			Expect(cursor2).To(Equal(2))
			Expect(cursor3).To(Equal(3))

			// The key difference: BuildConnection is 1 line vs manual boilerplate of 15+ lines
			// This achieves the 60-80% boilerplate reduction mentioned in the research
		})
	})
})
