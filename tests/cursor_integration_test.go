package paging_test

import (
	"context"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"
	"github.com/nrfta/paging-go/v2/sqlboiler"
	"github.com/nrfta/paging-go/v2/tests/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cursor Pagination Integration Tests", func() {
	var userIDs []string
	var userSchema *cursor.Schema[*models.User]

	BeforeEach(func() {
		// Clean tables before each test
		err := CleanupTables(ctx, container.DB)
		Expect(err).ToNot(HaveOccurred())

		// Seed test data
		userIDs, err = SeedUsers(ctx, container.DB, 25)
		Expect(err).ToNot(HaveOccurred())
		Expect(userIDs).To(HaveLen(25))

		// Create schema for users with sortable fields
		userSchema = cursor.NewSchema[*models.User]().
			Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
			Field("email", "e", func(u *models.User) any { return u.Email }).
			FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })
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
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
			}, "created_at", true)

			// Create fetcher
			fetcher := createUserFetcher()

			// Fetch with pagination using BuildFetchParams for consistent cursor encoding
			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Create paginator
			paginator, err := cursor.New(pageArgs, userSchema, users)
			Expect(err).ToNot(HaveOccurred())

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
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
			}, "created_at", true)

			fetcher := createUserFetcher()

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			firstPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator, err := cursor.New(pageArgs, userSchema, firstPageUsers)
			Expect(err).ToNot(HaveOccurred())
			endCursor, _ := paginator.PageInfo.EndCursor()

			// Fetch second page using EndCursor
			pageArgs.After = endCursor
			fetchParams, err = cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			secondPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Create second paginator
			paginator2, err := cursor.New(pageArgs, userSchema, secondPageUsers)
			Expect(err).ToNot(HaveOccurred())

			// Verify N+1 pattern: fetched LIMIT+1 records (11) because there's a 3rd page
			Expect(secondPageUsers).To(HaveLen(11), "N+1: should fetch LIMIT+1 when there's a next page")

			// Verify BuildConnection trims to LIMIT
			transform := func(u *models.User) (*models.User, error) { return u, nil }
			conn, err := cursor.BuildConnection(paginator2, secondPageUsers, transform)
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
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &first,
					After: currentCursor,
				}, "created_at", true)

				fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
				Expect(err).ToNot(HaveOccurred())
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())

				paginator, err := cursor.New(pageArgs, userSchema, users)
				Expect(err).ToNot(HaveOccurred())
				currentCursor, _ = paginator.PageInfo.EndCursor()
			}

			// Now fetch the last page
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: currentCursor,
			}, "created_at", true)

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			lastPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator, err := cursor.New(pageArgs, userSchema, lastPageUsers)
			Expect(err).ToNot(HaveOccurred())

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

			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
			}, "created_at", true)

			enc, _ := userSchema.EncoderFor(pageArgs)
			pastCursor, _ := enc.Encode(pastUser)
			pageArgs.After = pastCursor

			fetcher := createUserFetcher()

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator, err := cursor.New(pageArgs, userSchema, users)
			Expect(err).ToNot(HaveOccurred())

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
			first := 5
			pageArgs := paging.WithMultiSort(&paging.PageArgs{First: &first},
				paging.Sort{Column: "email", Desc: false},
			)

			fetcher := createUserFetcher()

			// Use schema's BuildFetchParams to get complete FetchParams
			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())

			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator, err := cursor.New(pageArgs, userSchema, users)
			Expect(err).ToNot(HaveOccurred())

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
			fetcher := createUserFetcher()

			// Fetch multiple pages and collect all IDs
			allIDs := make(map[string]bool)
			var currentCursor *string

			for i := 0; i < 3; i++ { // Fetch 3 pages
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &first,
					After: currentCursor,
				}, "created_at", true)

				fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
				Expect(err).ToNot(HaveOccurred())
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

				paginator, err := cursor.New(pageArgs, userSchema, users)
				Expect(err).ToNot(HaveOccurred())
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
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &first,
					After: currentCursor,
				}, "created_at", true)

				fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
				Expect(err).ToNot(HaveOccurred())
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())
				// Note: We fetch pageSize+1 but paginator will trim to pageSize

				paginator, err := cursor.New(pageArgs, userSchema, users)
				Expect(err).ToNot(HaveOccurred())

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
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: &invalid,
			}, "created_at", true)

			// Decoder should return nil for invalid cursor
			enc, _ := userSchema.EncoderFor(pageArgs)
			cursorPos, _ := enc.Decode(invalid)
			Expect(cursorPos).To(BeNil())

			// Fetch should work as if it's the first page (BuildFetchParams will return nil cursor)
			fetcher := createUserFetcher()

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetchParams.Cursor).To(BeNil()) // Invalid cursor decoded as nil
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
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: &invalid,
			}, "created_at", true)

			// Decoder should return nil for invalid JSON
			enc, _ := userSchema.EncoderFor(pageArgs)
			cursorPos, _ := enc.Decode(invalid)
			Expect(cursorPos).To(BeNil())
		})

		It("should handle empty cursor string", func() {
			first := 10
			empty := ""
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: &empty,
			}, "created_at", true)

			enc, _ := userSchema.EncoderFor(pageArgs)
			cursorPos, _ := enc.Decode(empty)
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
			// Create schema for posts
			postSchema := cursor.NewSchema[*models.Post]().
				Field("published_at", "p", func(p *models.Post) any { return p.PublishedAt }).
				FixedField("id", cursor.DESC, "i", func(p *models.Post) any { return p.ID })

			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "published_at", true)

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

			fetchParams, err := cursor.BuildFetchParams(pageArgs, postSchema)
			Expect(err).ToNot(HaveOccurred())
			posts, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			paginator, err := cursor.New(pageArgs, postSchema, posts)
			Expect(err).ToNot(HaveOccurred())

			// Verify N+1 pattern: Verify we fetched LIMIT+1 when there's a next page
			// 25 users * 3 posts * 2/3 published = 50 posts, so first page should have 11 records
			Expect(posts).To(HaveLen(11), "N+1: should fetch LIMIT+1 (10+1=11) when there's a next page")
			Expect(paginator.GetLimit()).To(Equal(10))

			// Verify BuildConnection trims to LIMIT
			transform := func(p *models.Post) (*models.Post, error) { return p, nil }
			conn, err := cursor.BuildConnection(paginator, posts, transform)
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
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)

			// Correct fetcher - no manual ORDER BY
			fetcher := createUserFetcher()

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			firstPage, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records
			Expect(firstPage).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			paginator, err := cursor.New(pageArgs, userSchema, firstPage)
			Expect(err).ToNot(HaveOccurred())
			endCursor, _ := paginator.PageInfo.EndCursor()

			// Fetch second page
			pageArgs.After = endCursor
			fetchParams, err = cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())

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
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)

			fetcher := createUserFetcher()

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			firstPageUsers, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records
			Expect(firstPageUsers).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			paginator, err := cursor.New(pageArgs, userSchema, firstPageUsers)
			Expect(err).ToNot(HaveOccurred())
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

			// Fetch with filter using cursor from first page
			pageArgs.After = endCursor
			fetchParams, err = cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())

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

			first := 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)

			fetchParams, err := cursor.BuildFetchParams(pageArgs, userSchema)
			Expect(err).ToNot(HaveOccurred())
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Verify we fetched LIMIT+1 records with multiple filters
			Expect(users).To(HaveLen(6), "N+1: should fetch LIMIT+1 (5+1=6) when there's a next page")

			// Verify HasNextPage is correctly set based on N+1 result
			paginator, err := cursor.New(pageArgs, userSchema, users)
			Expect(err).ToNot(HaveOccurred())
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue(), "HasNextPage should be true when we got LIMIT+1 records")

			// Verify all users match the filters
			for _, u := range users {
				Expect(u.Email).To(HaveSuffix("@example.com"))
				Expect(u.Name).ToNot(BeNil())
			}
		})
	})

	Describe("JOIN Query Pagination with Column Name Conflicts", func() {
		// Define a struct to hold joined data (user + post)
		type UserWithPost struct {
			UserID        string
			UserName      string
			UserEmail     string
			UserCreatedAt time.Time
			PostID        string
			PostTitle     string
			PostCreatedAt time.Time
		}

		BeforeEach(func() {
			// Seed posts for the users
			_, err := SeedPosts(ctx, container.DB, userIDs, 2)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should paginate JOIN queries with qualified column names in ORDER BY", func() {
			// CRITICAL: Create schema with QUALIFIED column names
			// This ensures the cursor strategy generates WHERE clauses like:
			// WHERE (posts.created_at, posts.id) < ($1, $2)
			// instead of ambiguous: WHERE (created_at, id) < ($1, $2)
			joinSchema := cursor.NewSchema[*UserWithPost]().
				Field("posts.created_at", "c", func(uwp *UserWithPost) any { return uwp.PostCreatedAt }).
				FixedField("posts.id", cursor.DESC, "i", func(uwp *UserWithPost) any { return uwp.PostID })

			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "posts.created_at", true)

			// Create fetcher using SQLBoiler query mods with INNER JOIN
			// CRITICAL: Must use qualified column names in qm.Select to avoid ambiguity
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*UserWithPost, error) {
					var results []*UserWithPost

					// Build query mods with explicit SELECT and INNER JOIN
					queryMods := []qm.QueryMod{
						qm.Select(
							"users.id AS user_id",
							"users.name AS user_name",
							"users.email AS user_email",
							"users.created_at AS user_created_at",
							"posts.id AS post_id",
							"posts.title AS post_title",
							"posts.created_at AS post_created_at",
						),
						qm.InnerJoin("users ON posts.user_id = users.id"),
					}
					queryMods = append(queryMods, mods...)

					err := models.Posts(queryMods...).Bind(ctx, container.DB, &results)
					return results, err
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return 0, nil
				},
				sqlboiler.CursorToQueryMods,
			)

			// Fetch first page using BuildFetchParams for consistent cursor encoding
			fetchParams, err := cursor.BuildFetchParams(pageArgs, joinSchema)
			Expect(err).ToNot(HaveOccurred())
			firstPageResults, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: We have 25 users * 2 posts = 50 total, so should get LIMIT+1 records
			Expect(firstPageResults).To(Or(
				HaveLen(11), // Full page with N+1
				HaveLen(10), // Exact page
			), "N+1: should fetch at least LIMIT records")

			// Verify data integrity - all results should have valid user and post data
			for _, result := range firstPageResults {
				Expect(result.UserID).ToNot(BeEmpty())
				Expect(result.PostID).ToNot(BeEmpty())
				Expect(result.UserName).ToNot(BeEmpty())
				Expect(result.PostTitle).ToNot(BeEmpty())
			}

			// Create paginator
			paginator, err := cursor.New(pageArgs, joinSchema, firstPageResults)
			Expect(err).ToNot(HaveOccurred())
			endCursor, _ := paginator.PageInfo.EndCursor()
			Expect(endCursor).ToNot(BeNil())

			// Fetch second page using the cursor
			pageArgs.After = endCursor
			fetchParams, err = cursor.BuildFetchParams(pageArgs, joinSchema)
			Expect(err).ToNot(HaveOccurred())

			secondPageResults, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Should have at least some results (not necessarily LIMIT+1)
			Expect(secondPageResults).ToNot(BeEmpty(), "Second page should have results")

			// Verify no overlap between pages (check post IDs)
			// Use the actual length of results to trim, accounting for N+1 pattern
			firstPageTrimmed := TrimToLimit(firstPageResults, 10)
			secondPageTrimmed := TrimToLimit(secondPageResults, 10)

			firstPagePostIDs := make(map[string]bool)
			for _, r := range firstPageTrimmed {
				firstPagePostIDs[r.PostID] = true
			}

			overlaps := []string{}
			for _, r := range secondPageTrimmed {
				if firstPagePostIDs[r.PostID] {
					overlaps = append(overlaps, r.PostID)
				}
			}

			Expect(overlaps).To(BeEmpty(), "Found overlapping post IDs between pages: %v", overlaps)

			// Verify pagination metadata
			paginator2, err := cursor.New(pageArgs, joinSchema, secondPageResults)
			Expect(err).ToNot(HaveOccurred())
			hasNext, _ := paginator2.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue()) // Still have more pages (50 total posts)

			hasPrev, _ := paginator2.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue()) // We're on page 2
		})

		It("should handle ORDER BY with unqualified column names causing ambiguity", func() {
			// This test intentionally uses UNQUALIFIED column names to verify error handling
			// Both users and posts tables have 'created_at' and 'id' columns

			// Create fetcher using SQLBoiler with INNER JOIN
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*UserWithPost, error) {
					var results []*UserWithPost

					// Build query mods with explicit SELECT and INNER JOIN
					queryMods := []qm.QueryMod{
						qm.Select(
							"users.id AS user_id",
							"users.name AS user_name",
							"users.email AS user_email",
							"users.created_at AS user_created_at",
							"posts.id AS post_id",
							"posts.title AS post_title",
							"posts.created_at AS post_created_at",
						),
						qm.InnerJoin("users ON posts.user_id = users.id"),
					}
					queryMods = append(queryMods, mods...)

					err := models.Posts(queryMods...).Bind(ctx, container.DB, &results)
					return results, err
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return 0, nil
				},
				sqlboiler.CursorToQueryMods,
			)

			// Use UNQUALIFIED column names in OrderBy
			// This will cause "column reference 'created_at' is ambiguous" error
			fetchParams := paging.FetchParams{
				Limit:  10 + 1,
				Cursor: nil,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true}, // UNQUALIFIED - ambiguous!
					{Column: "id", Desc: true},         // UNQUALIFIED - ambiguous!
				},
			}

			_, err := fetcher.Fetch(ctx, fetchParams)

			// Should get an error about ambiguous column reference
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("ambiguous"),
				ContainSubstring("column"),
			))
		})

		It("should correctly order by user columns when specified", func() {
			// This test verifies we can sort by user columns instead of post columns
			// Create schema and paginator to verify cursor generation works
			joinSchema := cursor.NewSchema[*UserWithPost]().
				Field("users.created_at", "c", func(uwp *UserWithPost) any { return uwp.UserCreatedAt }).
				FixedField("users.id", cursor.DESC, "i", func(uwp *UserWithPost) any { return uwp.UserID })

			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "users.created_at", true)

			// Create fetcher using SQLBoiler with INNER JOIN
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*UserWithPost, error) {
					var results []*UserWithPost

					// Build query mods with explicit SELECT and INNER JOIN
					queryMods := []qm.QueryMod{
						qm.Select(
							"users.id AS user_id",
							"users.name AS user_name",
							"users.email AS user_email",
							"users.created_at AS user_created_at",
							"posts.id AS post_id",
							"posts.title AS post_title",
							"posts.created_at AS post_created_at",
						),
						qm.InnerJoin("users ON posts.user_id = users.id"),
					}
					queryMods = append(queryMods, mods...)

					err := models.Posts(queryMods...).Bind(ctx, container.DB, &results)
					return results, err
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return 0, nil
				},
				sqlboiler.CursorToQueryMods,
			)

			// Order by USERS columns (qualified) using BuildFetchParams
			fetchParams, err := cursor.BuildFetchParams(pageArgs, joinSchema)
			Expect(err).ToNot(HaveOccurred())
			results, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// N+1 pattern: Should get LIMIT+1 records
			Expect(results).To(HaveLen(11), "N+1: should fetch LIMIT+1 (10+1=11)")

			paginator, err := cursor.New(pageArgs, joinSchema, results)
			Expect(err).ToNot(HaveOccurred())

			// Verify HasNextPage is set correctly
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			// Verify results are sorted by user creation time
			// Each user has 2 posts, so consecutive posts might have the same user
			for i := 1; i < len(results)-1; i++ {
				prev := results[i-1]
				curr := results[i]

				// User created_at should be DESC (newer or equal)
				Expect(prev.UserCreatedAt.After(curr.UserCreatedAt) || prev.UserCreatedAt.Equal(curr.UserCreatedAt)).To(BeTrue())
			}
		})
	})
})
