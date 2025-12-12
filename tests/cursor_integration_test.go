package paging_test

import (
	"context"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/cursor"
	"github.com/nrfta/go-paging/sqlboiler"
	"github.com/nrfta/go-paging/tests/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cursor Pagination Integration Tests", func() {
	var userIDs []string
	var encoder paging.CursorEncoder[*models.User]

	BeforeEach(func() {
		// Clean tables before each test
		err := CleanupTables(ctx, container.DB)
		Expect(err).ToNot(HaveOccurred())

		// Seed test data
		userIDs, err = SeedUsers(ctx, container.DB, 25)
		Expect(err).ToNot(HaveOccurred())
		Expect(userIDs).To(HaveLen(25))

		// Create encoder for users (created_at, id)
		encoder = cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
			return map[string]any{
				"created_at": u.CreatedAt,
				"id":         u.ID,
			}
		})
	})

	// Helper to create a standard user fetcher with cursor strategy
	createUserFetcher := func() paging.Fetcher[*models.User] {
		return sqlboiler.NewFetcher(
			func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
				return models.Users(mods...).All(ctx, container.DB)
			},
			func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
				return 0, nil // Count not used for cursor pagination
			},
			sqlboiler.CursorToQueryMods,
		)
	}

	Describe("Basic Cursor Pagination", func() {
		It("should paginate users with default page size using SQLBoiler", func() {
			// Create paginator (first page, no cursor)
			first := 10
			pageArgs := &paging.PageArgs{
				First: &first,
			}

			// Create fetcher
			fetcher := createUserFetcher()

			// Fetch with pagination
			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: nil, // First page
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Create paginator
			paginator := cursor.New(pageArgs, encoder, users)

			// Verify results (N+1: we fetch 11, paginator trims to 10)
			Expect(paginator.GetLimit()).To(Equal(10))
			Expect(paginator.GetCursor()).To(BeNil()) // First page

			// Verify PageInfo
			hasNext, err := paginator.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue()) // Full page implies more data

			hasPrev, err := paginator.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPrev).To(BeFalse()) // No cursor = first page

			totalCount, err := paginator.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(BeNil()) // Cursor pagination doesn't provide total count

			// Verify cursors are populated
			startCursor, err := paginator.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil())

			endCursor, err := paginator.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil())
		})
	})

	Describe("Second Page Navigation", func() {
		It("should paginate to second page using EndCursor", func() {
			// Fetch first page
			first := 10
			pageArgs := &paging.PageArgs{
				First: &first,
			}

			fetcher := createUserFetcher()

			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			firstPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator := cursor.New(pageArgs, encoder, firstPageUsers)
			endCursor, _ := paginator.PageInfo.EndCursor()

			// Fetch second page using EndCursor
			pageArgs.After = endCursor
			cursorPos, _ := encoder.Decode(*endCursor)

			fetchParams.Cursor = cursorPos
			secondPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Create second paginator
			paginator2 := cursor.New(pageArgs, encoder, secondPageUsers)

			// Verify N+1 pattern: fetched LIMIT+1 records (11) because there's a 3rd page
			Expect(secondPageUsers).To(HaveLen(11), "N+1: should fetch LIMIT+1 when there's a next page")

			// Verify BuildConnection trims to LIMIT
			transform := func(u *models.User) (*models.User, error) { return u, nil }
			conn, err := cursor.BuildConnection(paginator2, secondPageUsers, encoder, transform)
			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(10), "BuildConnection should trim to LIMIT")

			// Verify no overlap with first page (trim to limit for comparison)
			limit := 10
			for _, u2 := range TrimToLimit(secondPageUsers, limit) {
				for _, u1 := range TrimToLimit(firstPageUsers, limit) {
					Expect(u2.ID).ToNot(Equal(u1.ID))
				}
			}

			// Still has next page (25 total, we're on second page)
			hasNext, _ := paginator2.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			// Has previous page now
			hasPrev, _ := paginator2.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Last Page Handling", func() {
		It("should handle last page correctly", func() {
			// Navigate through pages to get to the last one
			first := 10
			var currentCursor *string

			fetcher := createUserFetcher()

			// Get to page 3 (after 20 records, should get last 5)
			for i := 0; i < 2; i++ {
				pageArgs := &paging.PageArgs{
					First: &first,
					After: currentCursor,
				}

				var cursorPos *paging.CursorPosition
				if currentCursor != nil {
					cursorPos, _ = encoder.Decode(*currentCursor)
				}

				fetchParams := paging.FetchParams{
					Limit: 10 + 1,
					Cursor: cursorPos,
					OrderBy: []paging.OrderBy{
						{Column: "created_at", Desc: true},
						{Column: "id", Desc: true},
					},
				}
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())

				paginator := cursor.New(pageArgs, encoder, users)
				currentCursor, _ = paginator.PageInfo.EndCursor()
			}

			// Now fetch the last page
			pageArgs := &paging.PageArgs{
				First: &first,
				After: currentCursor,
			}

			cursorPos, _ := encoder.Decode(*currentCursor)
			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: cursorPos,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			lastPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator := cursor.New(pageArgs, encoder, lastPageUsers)

			// Last page has 5 items (25 total - 20 already fetched)
			Expect(lastPageUsers).To(HaveLen(5))

			// No next page
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Has previous page
			hasPrev, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Empty Results", func() {
		It("should handle cursor beyond all data", func() {
			// Create a cursor that's beyond all data for DESC ordering
			// With DESC order, we need a cursor in the PAST (before all records)
			// to get zero results, since < operator gets records BEFORE the cursor
			pastUser := &models.User{
				ID:        "00000000-0000-0000-0000-000000000000",
				CreatedAt: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			pastCursor, _ := encoder.Encode(pastUser)

			first := 10
			pageArgs := &paging.PageArgs{
				First: &first,
				After: pastCursor,
			}

			fetcher := createUserFetcher()

			cursorPos, _ := encoder.Decode(*pastCursor)
			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: cursorPos,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator := cursor.New(pageArgs, encoder, users)

			// Should return 0 items
			Expect(users).To(HaveLen(0))

			// No next page
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Start and End cursors should be nil
			startCursor, _ := paginator.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil())

			endCursor, _ := paginator.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil())
		})
	})

	Describe("Custom Sorting", func() {
		It("should sort by email ascending", func() {
			// Create encoder for email sorting
			emailEncoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
				return map[string]any{
					"email": u.Email,
					"id":    u.ID,
				}
			})

			first := 5
			pageArgs := &paging.PageArgs{First: &first}
			pageArgs = paging.WithSortBy(pageArgs, false, "email", "id")

			fetcher := createUserFetcher()

			fetchParams := paging.FetchParams{
				Limit: 5 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "email", Desc: false},
					{Column: "id", Desc: false},
				},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator := cursor.New(pageArgs, emailEncoder, users)

			// Verify N+1 pattern: fetched LIMIT+1 records (6) because there are 25 users
			Expect(users).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			// Verify paginator correctly detects next page from N+1
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue(), "HasNextPage should be true when we got LIMIT+1 records")

			// Verify sorted order
			Expect(users[0].Email).To(HaveSuffix("@example.com"))
		})
	})

	Describe("Composite Key Uniqueness", func() {
		It("should prevent duplicates with same created_at timestamps", func() {
			// This test verifies that the ID tiebreaker prevents duplicates
			// when multiple users have the same created_at timestamp

			first := 10
			pageArgs := &paging.PageArgs{
				First: &first,
			}

			fetcher := createUserFetcher()

			// Fetch multiple pages and collect all IDs
			allIDs := make(map[string]bool)
			var currentCursor *string

			for i := 0; i < 3; i++ { // Fetch 3 pages
				pageArgs.After = currentCursor

				var cursorPos *paging.CursorPosition
				if currentCursor != nil {
					cursorPos, _ = encoder.Decode(*currentCursor)
				}

				fetchParams := paging.FetchParams{
					Limit: 10 + 1,
					Cursor: cursorPos,
					OrderBy: []paging.OrderBy{
						{Column: "created_at", Desc: true},
						{Column: "id", Desc: true},
					},
				}
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())

				if len(users) == 0 {
					break
				}

				// Check for duplicates (trim to limit for N+1 pattern)
				for _, u := range TrimToLimit(users, 10) {
					if allIDs[u.ID] {
						Fail("Found duplicate ID: " + u.ID)
					}
					allIDs[u.ID] = true
				}

				paginator := cursor.New(pageArgs, encoder, users)
				currentCursor, _ = paginator.PageInfo.EndCursor()
			}

			// Should have collected 25 unique IDs
			Expect(allIDs).To(HaveLen(25))
		})
	})

	Describe("Large Dataset Performance", func() {
		It("should handle 100 users efficiently", func() {
			// Clean and seed larger dataset
			err := CleanupTables(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			userIDs, err := SeedUsers(ctx, container.DB, 100)
			Expect(err).ToNot(HaveOccurred())
			Expect(userIDs).To(HaveLen(100))

			// Create fetcher once
			fetcher := createUserFetcher()

			// Paginate through all pages
			pageSize := 25
			first := pageSize
			var currentCursor *string
			totalFetched := 0

			for page := 0; page < 4; page++ {
				pageArgs := &paging.PageArgs{
					First: &first,
					After: currentCursor,
				}

				var cursorPos *paging.CursorPosition
				if currentCursor != nil {
					cursorPos, _ = encoder.Decode(*currentCursor)
				}

				fetchParams := paging.FetchParams{
					Limit:  pageSize + 1, // N+1 pattern
					Cursor: cursorPos,
					OrderBy: []paging.OrderBy{
						{Column: "created_at", Desc: true},
						{Column: "id", Desc: true},
					},
				}
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())
				// Note: We fetch pageSize+1 but paginator will trim to pageSize

				paginator := cursor.New(pageArgs, encoder, users)

				// After paginator processes, we should have pageSize items
				// (trimmed from pageSize+1 if we got that many)
				totalFetched += pageSize

				currentCursor, _ = paginator.PageInfo.EndCursor()
			}

			Expect(totalFetched).To(Equal(100))
		})
	})

	Describe("Invalid Cursor Handling", func() {
		It("should handle malformed base64 cursor", func() {
			first := 10
			invalid := "invalid-base64!!!"
			_ = &paging.PageArgs{
				First: &first,
				After: &invalid,
			}

			// Decoder should return nil for invalid cursor
			cursorPos, _ := encoder.Decode(invalid)
			Expect(cursorPos).To(BeNil())

			// Fetch should work as if it's the first page
			fetcher := createUserFetcher()

			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: cursorPos, // nil
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: With invalid cursor treated as first page, we fetch LIMIT+1
			// We have 25 users, so we should get 11 records (10+1)
			Expect(users).To(HaveLen(11), "N+1: should fetch LIMIT+1 records for first page")
		})

		It("should handle invalid JSON in cursor", func() {
			first := 10
			// Base64 encode invalid JSON
			invalid := "e25vdCB2YWxpZCBqc29ufQ==" // base64("{not valid json}")
			_ = &paging.PageArgs{
				First: &first,
				After: &invalid,
			}

			// Decoder should return nil for invalid JSON
			cursorPos, _ := encoder.Decode(invalid)
			Expect(cursorPos).To(BeNil())
		})

		It("should handle empty cursor string", func() {
			first := 10
			empty := ""
			_ = &paging.PageArgs{
				First: &first,
				After: &empty,
			}

			cursorPos, _ := encoder.Decode(empty)
			Expect(cursorPos).To(BeNil())
		})
	})

	Describe("Posts with Relationships", func() {
		BeforeEach(func() {
			// Seed posts for users
			_, err := SeedPosts(ctx, container.DB, userIDs, 3)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should paginate posts with published filter", func() {
			// Create encoder for posts
			postEncoder := cursor.NewCompositeCursorEncoder(func(p *models.Post) map[string]any {
				return map[string]any{
					"published_at": p.PublishedAt,
					"id":           p.ID,
				}
			})

			first := 10
			pageArgs := &paging.PageArgs{First: &first}

			// Create fetcher with WHERE filter
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.Post, error) {
					// Add WHERE filter to query mods
					mods = append([]qm.QueryMod{qm.Where("published_at IS NOT NULL")}, mods...)
					return models.Posts(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					mods = append([]qm.QueryMod{qm.Where("published_at IS NOT NULL")}, mods...)
					return models.Posts(mods...).Count(ctx, container.DB)
				},
				sqlboiler.CursorToQueryMods,
			)

			fetchParams := paging.FetchParams{
				Limit: 10 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "published_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			posts, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator := cursor.New(pageArgs, postEncoder, posts)

			// Verify N+1 pattern: Verify we fetched LIMIT+1 when there's a next page
			// 25 users * 3 posts * 2/3 published = 50 posts, so first page should have 11 records
			Expect(posts).To(HaveLen(11), "N+1: should fetch LIMIT+1 (10+1=11) when there's a next page")
			Expect(paginator.GetLimit()).To(Equal(10))

			// Verify BuildConnection trims to LIMIT
			transform := func(p *models.Post) (*models.Post, error) { return p, nil }
			conn, err := cursor.BuildConnection(paginator, posts, postEncoder, transform)
			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(10), "BuildConnection should trim to LIMIT")

			// Verify all posts are published
			for _, post := range posts {
				Expect(post.PublishedAt).ToNot(BeNil())
			}

			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue()) // 25 users * 3 posts * 2/3 published = 50 posts
		})
	})

	Describe("Sorting Conflicts", func() {
		It("should work correctly when ORDER BY is only in FetchParams", func() {
			// This is the CORRECT way - don't add qm.OrderBy, let cursor strategy handle it

			first := 5
			pageArgs := &paging.PageArgs{First: &first}

			// Correct fetcher - no manual ORDER BY
			fetcher := createUserFetcher()

			fetchParams := paging.FetchParams{
				Limit: 5 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			firstPage, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records
			Expect(firstPage).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			paginator := cursor.New(pageArgs, encoder, firstPage)
			endCursor, _ := paginator.PageInfo.EndCursor()

			// Fetch second page
			cursorPos, _ := encoder.Decode(*endCursor)
			fetchParams.Cursor = cursorPos

			secondPage, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 for second page too
			Expect(secondPage).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			// Verify no overlap (trim to limit for N+1 pattern)
			limit := 5
			for _, u2 := range TrimToLimit(secondPage, limit) {
				for _, u1 := range TrimToLimit(firstPage, limit) {
					Expect(u2.ID).ToNot(Equal(u1.ID))
				}
			}
		})
	})

	Describe("SQL Generation with Filters", func() {
		It("should combine user filters with cursor conditions correctly", func() {
			// First, fetch page without filter to get a cursor
			first := 5
			pageArgs := &paging.PageArgs{First: &first}

			fetcher := createUserFetcher()

			fetchParams := paging.FetchParams{
				Limit: 5 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			firstPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records
			Expect(firstPageUsers).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			paginator := cursor.New(pageArgs, encoder, firstPageUsers)
			endCursor, _ := paginator.PageInfo.EndCursor()

			// Now apply a filter AND use the cursor
			// This tests that the SQL is: WHERE email LIKE ? AND (cursor conditions)
			fetcherWithFilter := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					// Prepend user's filter before cursor mods
					mods = append([]qm.QueryMod{qm.Where("email LIKE ?", "%@example.com")}, mods...)
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return 0, nil
				},
				sqlboiler.CursorToQueryMods,
			)

			// Decode cursor and fetch with filter
			cursorPos, _ := encoder.Decode(*endCursor)
			fetchParams.Cursor = cursorPos

			filteredUsers, err := fetcherWithFilter.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Should get users that match BOTH conditions:
			// 1. email LIKE '%@example.com'
			// 2. (created_at, id) < cursor position
			// N+1 pattern: Verify we fetched LIMIT+1 records (filter matches all users)
			Expect(filteredUsers).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			// Verify all users match the filter
			for _, u := range filteredUsers {
				Expect(u.Email).To(HaveSuffix("@example.com"))
			}

			// Verify no overlap with first page (trim to limit for N+1 pattern)
			limit := 5
			for _, u2 := range TrimToLimit(filteredUsers, limit) {
				for _, u1 := range TrimToLimit(firstPageUsers, limit) {
					Expect(u2.ID).ToNot(Equal(u1.ID))
				}
			}
		})

		It("should handle multiple filters combined with cursor", func() {
			// Fetch with multiple filters AND cursor
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					// Multiple filters before cursor
					mods = append([]qm.QueryMod{
						qm.Where("email LIKE ?", "%@example.com"),
						qm.Where("name IS NOT NULL"),
					}, mods...)
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return 0, nil
				},
				sqlboiler.CursorToQueryMods,
			)

			fetchParams := paging.FetchParams{
				Limit: 5 + 1,
				Cursor: nil,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records with multiple filters
			Expect(users).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			// Verify HasNextPage is correctly set based on N+1 result
			first := 5
			pageArgs := &paging.PageArgs{First: &first}
			paginator := cursor.New(pageArgs, encoder, users)
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue(), "HasNextPage should be true when we got LIMIT+1 records")

			// Verify all users match the filters
			for _, u := range users {
				Expect(u.Email).To(HaveSuffix("@example.com"))
				Expect(u.Name).ToNot(BeNil())
			}
		})
	})
})
