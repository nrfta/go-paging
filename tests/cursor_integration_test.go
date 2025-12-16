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
	var userPaginator paging.Paginator[*models.User]

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

		// Create paginator (reusable)
		fetcher := createUserFetcher()
		userPaginator = cursor.New(fetcher, userSchema)
	})

	Describe("Basic Cursor Pagination", func() {
		It("should paginate users with default page size using SQLBoiler", func() {
			// Create paginator (first page, no cursor)
			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
			}, "created_at", true)

			// Paginate
			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Verify results
			Expect(page.Nodes).To(HaveLen(10))
			Expect(page.Metadata.Strategy).To(Equal("cursor"))

			// Verify PageInfo
			hasNext, err := page.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue()) // Full page implies more data

			hasPrev, err := page.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPrev).To(BeFalse()) // No cursor = first page

			totalCount, err := page.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(BeNil()) // Cursor pagination doesn't provide total count

			// Verify cursors are populated
			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil())

			endCursor, err := page.PageInfo.EndCursor()
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

			firstPage, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(firstPage.Nodes).To(HaveLen(10))
			endCursor, _ := firstPage.PageInfo.EndCursor()

			// Fetch second page using EndCursor
			pageArgs.After = endCursor
			secondPage, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Verify BuildConnection works
			transform := func(u *models.User) (*models.User, error) { return u, nil }
			conn, err := cursor.BuildConnection(secondPage, userSchema, pageArgs, transform)
			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(10))

			// Verify no overlap with first page
			for _, u2 := range secondPage.Nodes {
				for _, u1 := range firstPage.Nodes {
					Expect(u2.ID).ToNot(Equal(u1.ID))
				}
			}

			// Still has next page (25 total, we're on second page)
			hasNext, _ := secondPage.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			// Has previous page now
			hasPrev, _ := secondPage.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Last Page Handling", func() {
		It("should handle last page correctly", func() {
			// Navigate through pages to get to the last one
			first := 10
			var currentCursor *string

			// Get to page 3 (after 20 records, should get last 5)
			for i := 0; i < 2; i++ {
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &first,
					After: currentCursor,
				}, "created_at", true)

				page, err := userPaginator.Paginate(ctx, pageArgs)
				Expect(err).ToNot(HaveOccurred())
				currentCursor, _ = page.PageInfo.EndCursor()
			}

			// Now fetch the last page
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: currentCursor,
			}, "created_at", true)

			lastPage, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Last page has 5 items (25 total - 20 already fetched)
			Expect(lastPage.Nodes).To(HaveLen(5))

			// No next page
			hasNext, _ := lastPage.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Has previous page
			hasPrev, _ := lastPage.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Empty Results", func() {
		It("should handle cursor beyond all data", func() {
			// Create a cursor that's beyond all data for DESC ordering
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

			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should return 0 items
			Expect(page.Nodes).To(HaveLen(0))

			// No next page
			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Start and End cursors should be nil
			startCursor, _ := page.PageInfo.StartCursor()
			Expect(startCursor).To(BeNil())

			endCursor, _ := page.PageInfo.EndCursor()
			Expect(endCursor).To(BeNil())
		})
	})

	Describe("Custom Sorting", func() {
		It("should sort by email ascending", func() {
			first := 5
			pageArgs := paging.WithMultiSort(&paging.PageArgs{First: &first},
				paging.Sort{Column: "email", Desc: false},
			)

			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get 5 users
			Expect(page.Nodes).To(HaveLen(5))

			// Paginator correctly detects next page
			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			// Verify sorted order
			Expect(page.Nodes[0].Email).To(HaveSuffix("@example.com"))
		})
	})

	Describe("Composite Key Uniqueness", func() {
		It("should prevent duplicates with same created_at timestamps", func() {
			first := 10

			// Fetch multiple pages and collect all IDs
			allIDs := make(map[string]bool)
			var currentCursor *string

			for i := 0; i < 3; i++ { // Fetch 3 pages
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &first,
					After: currentCursor,
				}, "created_at", true)

				page, err := userPaginator.Paginate(ctx, pageArgs)
				Expect(err).ToNot(HaveOccurred())

				if len(page.Nodes) == 0 {
					break
				}

				// Check for duplicates
				for _, u := range page.Nodes {
					if allIDs[u.ID] {
						Fail("Found duplicate ID: " + u.ID)
					}
					allIDs[u.ID] = true
				}

				currentCursor, _ = page.PageInfo.EndCursor()
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

			// Create new paginator for larger dataset
			fetcher := createUserFetcher()
			paginator := cursor.New(fetcher, userSchema)

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

				result, err := paginator.Paginate(ctx, pageArgs)
				Expect(err).ToNot(HaveOccurred())

				totalFetched += len(result.Nodes)
				currentCursor, _ = result.PageInfo.EndCursor()
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

			// Paginate should work as if it's the first page
			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get first page results
			Expect(page.Nodes).To(HaveLen(10))
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

			paginator := cursor.New(fetcher, postSchema)
			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get 10 posts
			Expect(page.Nodes).To(HaveLen(10))

			// Verify BuildConnection works
			transform := func(p *models.Post) (*models.Post, error) { return p, nil }
			conn, err := cursor.BuildConnection(page, postSchema, pageArgs, transform)
			Expect(err).ToNot(HaveOccurred())
			Expect(conn.Nodes).To(HaveLen(10))

			// Verify all posts are published
			for _, post := range page.Nodes {
				Expect(post.PublishedAt).ToNot(BeNil())
			}

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue()) // 25 users * 3 posts * 2/3 published = 50 posts
		})
	})

	Describe("SQL Generation with Filters", func() {
		It("should combine user filters with cursor conditions correctly", func() {
			// First, fetch page without filter to get a cursor
			first := 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)

			firstPage, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(firstPage.Nodes).To(HaveLen(5))
			endCursor, _ := firstPage.PageInfo.EndCursor()

			// Now apply a filter AND use the cursor
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

			paginatorWithFilter := cursor.New(fetcherWithFilter, userSchema)

			// Fetch with filter using cursor from first page
			pageArgs.After = endCursor
			filteredPage, err := paginatorWithFilter.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get users that match BOTH conditions
			Expect(filteredPage.Nodes).To(HaveLen(5))

			// Verify all users match the filter
			for _, u := range filteredPage.Nodes {
				Expect(u.Email).To(HaveSuffix("@example.com"))
			}

			// Verify no overlap with first page
			for _, u2 := range filteredPage.Nodes {
				for _, u1 := range firstPage.Nodes {
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

			paginator := cursor.New(fetcher, userSchema)

			first := 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)

			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get 5 users with multiple filters
			Expect(page.Nodes).To(HaveLen(5))

			// Verify HasNextPage is correctly set
			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			// Verify all users match the filters
			for _, u := range page.Nodes {
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
			// Create schema with QUALIFIED column names
			joinSchema := cursor.NewSchema[*UserWithPost]().
				Field("posts.created_at", "c", func(uwp *UserWithPost) any { return uwp.PostCreatedAt }).
				FixedField("posts.id", cursor.DESC, "i", func(uwp *UserWithPost) any { return uwp.PostID })

			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "posts.created_at", true)

			// Create fetcher using SQLBoiler query mods with INNER JOIN
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

			paginator := cursor.New(fetcher, joinSchema)

			// Fetch first page
			firstPage, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should get results
			Expect(firstPage.Nodes).To(HaveLen(10))

			// Verify data integrity
			for _, result := range firstPage.Nodes {
				Expect(result.UserID).ToNot(BeEmpty())
				Expect(result.PostID).ToNot(BeEmpty())
				Expect(result.UserName).ToNot(BeEmpty())
				Expect(result.PostTitle).ToNot(BeEmpty())
			}

			endCursor, _ := firstPage.PageInfo.EndCursor()
			Expect(endCursor).ToNot(BeNil())

			// Fetch second page using the cursor
			pageArgs.After = endCursor
			secondPage, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Should have at least some results
			Expect(secondPage.Nodes).ToNot(BeEmpty())

			// Verify no overlap between pages
			firstPagePostIDs := make(map[string]bool)
			for _, r := range firstPage.Nodes {
				firstPagePostIDs[r.PostID] = true
			}

			overlaps := []string{}
			for _, r := range secondPage.Nodes {
				if firstPagePostIDs[r.PostID] {
					overlaps = append(overlaps, r.PostID)
				}
			}

			Expect(overlaps).To(BeEmpty(), "Found overlapping post IDs between pages")

			// Verify pagination metadata
			hasNext, _ := secondPage.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			hasPrev, _ := secondPage.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})
})
