package paging_test

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/offset"
	"github.com/nrfta/paging-go/v2/sqlboiler"
	"github.com/nrfta/paging-go/v2/tests/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Offset Pagination Integration Tests", func() {
	var userIDs []string
	var userPaginator paging.Paginator[*models.User]

	// Helper to create a standard user fetcher with offset strategy
	createUserFetcher := func() paging.Fetcher[*models.User] {
		return sqlboiler.NewFetcher(
			func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
				return models.Users(mods...).All(ctx, container.DB)
			},
			func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
				return models.Users(mods...).Count(ctx, container.DB)
			},
			sqlboiler.OffsetToQueryMods,
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

		// Create paginator (reusable)
		fetcher := createUserFetcher()
		userPaginator = offset.New(fetcher)
	})

	Describe("Basic Offset Pagination", func() {
		It("should paginate users with default page size using SQLBoiler", func() {
			// Create paginator (first page)
			first := 10
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
			}, "created_at", true)

			// Paginate
			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Verify results
			Expect(page.Nodes).To(HaveLen(10))
			Expect(page.Metadata.Strategy).To(Equal("offset"))

			// Verify PageInfo
			hasNext, err := page.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue())

			hasPrev, err := page.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPrev).To(BeFalse())

			total, err := page.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(*total).To(Equal(25))
		})

		It("should paginate to second page", func() {
			// Create second page cursor
			first := 10
			cursor := offset.EncodeCursor(10) // After first 10 records
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: cursor,
			}, "created_at", true)

			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Verify
			Expect(page.Nodes).To(HaveLen(10))

			// Still has next page (25 total, we're at 10-19)
			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			hasPrev, _ := page.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})

		It("should handle last page correctly", func() {
			// Go to last page
			first := 10
			cursor := offset.EncodeCursor(20) // After 20 records, should get last 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: cursor,
			}, "created_at", true)

			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			// Last page has 5 items (25 total - 20 offset)
			Expect(page.Nodes).To(HaveLen(5))

			// No next page
			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Has previous page
			hasPrev, _ := page.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Custom Sorting", func() {
		It("should sort by email ascending", func() {
			// Sort by email
			first := 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, "email", false)

			page, err := userPaginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(page.Nodes).To(HaveLen(5))
			// Verify sorted order (user1@, user10@, user11@, ...)
			Expect(page.Nodes[0].Email).To(HaveSuffix("@example.com"))
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
			paginator := offset.New(fetcher)

			// Paginate through all pages
			pageSize := 25
			var currentCursor *string

			for page := 0; page < 4; page++ {
				pageArgs := paging.WithSortBy(&paging.PageArgs{
					First: &pageSize,
					After: currentCursor,
				}, "created_at", true)

				result, err := paginator.Paginate(ctx, pageArgs)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Nodes).To(HaveLen(25))

				// Advance cursor for next page
				if page < 3 {
					nextCursor := offset.EncodeCursor((page + 1) * pageSize)
					currentCursor = nextCursor
				}
			}
		})
	})

	Describe("Posts with Relationships", func() {
		BeforeEach(func() {
			// Seed posts for users
			_, err := SeedPosts(ctx, container.DB, userIDs, 3)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should paginate posts with published filter", func() {
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
				sqlboiler.OffsetToQueryMods,
			)

			paginator := offset.New(fetcher)
			page, err := paginator.Paginate(ctx, pageArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(page.Nodes).To(HaveLen(10))
			// Verify all posts are published
			for _, post := range page.Nodes {
				Expect(post.PublishedAt).ToNot(BeNil())
			}

			hasNext, _ := page.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue()) // 25 users * 3 posts * 2/3 published = 50 posts
		})
	})
})
