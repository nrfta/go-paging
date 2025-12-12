package paging_test

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/offset"
	"github.com/nrfta/go-paging/sqlboiler"
	"github.com/nrfta/go-paging/tests/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Offset Pagination Integration Tests", func() {
	var userIDs []string

	BeforeEach(func() {
		// Clean tables before each test
		err := CleanupTables(ctx, container.DB)
		Expect(err).ToNot(HaveOccurred())

		// Seed test data
		userIDs, err = SeedUsers(ctx, container.DB, 25)
		Expect(err).ToNot(HaveOccurred())
		Expect(userIDs).To(HaveLen(25))
	})

	Describe("Basic Offset Pagination", func() {
		It("should paginate users with default page size using SQLBoiler", func() {
			// Get total count using SQLBoiler
			totalCount, err := models.Users().Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(int64(25)))

			// Create paginator (first page)
			first := 10
			pageArgs := &paging.PageArgs{
				First: &first,
			}
			paginator := offset.New(pageArgs, totalCount)

			// Create SQLBoiler fetcher with offset strategy
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return models.Users(mods...).Count(ctx, container.DB)
				},
				sqlboiler.OffsetToQueryMods,
			)

			// Fetch with pagination
			fetchParams := paging.FetchParams{
				Offset:  paginator.Offset,
				Limit:   paginator.Limit,
				OrderBy: []paging.OrderBy{{Column: "created_at", Desc: true}},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Verify results
			Expect(users).To(HaveLen(10))
			Expect(paginator.Limit).To(Equal(10))
			Expect(paginator.Offset).To(Equal(0))

			// Verify PageInfo
			hasNext, err := paginator.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue())

			hasPrev, err := paginator.PageInfo.HasPreviousPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasPrev).To(BeFalse())

			total, err := paginator.PageInfo.TotalCount()
			Expect(err).ToNot(HaveOccurred())
			Expect(*total).To(Equal(25))
		})

		It("should paginate to second page", func() {
			totalCount, err := models.Users().Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			// Create second page cursor
			first := 10
			cursor := offset.EncodeCursor(10) // After first 10 records
			pageArgs := &paging.PageArgs{
				First: &first,
				After: cursor,
			}
			paginator := offset.New(pageArgs, totalCount)

			// Create fetcher and fetch
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return models.Users(mods...).Count(ctx, container.DB)
				},
				sqlboiler.OffsetToQueryMods,
			)

			fetchParams := paging.FetchParams{
				Offset:  paginator.Offset,
				Limit:   paginator.Limit,
				OrderBy: []paging.OrderBy{{Column: "created_at", Desc: true}},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Verify
			Expect(users).To(HaveLen(10))
			Expect(paginator.Offset).To(Equal(10))

			// Still has next page (25 total, we're at 10-19)
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue())

			hasPrev, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})

		It("should handle last page correctly", func() {
			totalCount, err := models.Users().Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			// Go to last page
			first := 10
			cursor := offset.EncodeCursor(20) // After 20 records, should get last 5
			pageArgs := &paging.PageArgs{
				First: &first,
				After: cursor,
			}
			paginator := offset.New(pageArgs, totalCount)

			// Create fetcher and fetch
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return models.Users(mods...).Count(ctx, container.DB)
				},
				sqlboiler.OffsetToQueryMods,
			)

			fetchParams := paging.FetchParams{
				Offset:  paginator.Offset,
				Limit:   paginator.Limit,
				OrderBy: []paging.OrderBy{{Column: "created_at", Desc: true}},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			// Last page has 5 items (25 total - 20 offset)
			Expect(users).To(HaveLen(5))

			// No next page
			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeFalse())

			// Has previous page
			hasPrev, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPrev).To(BeTrue())
		})
	})

	Describe("Custom Sorting", func() {
		It("should sort by email ascending", func() {
			totalCount, err := models.Users().Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			// Sort by email
			first := 5
			pageArgs := paging.WithSortBy(&paging.PageArgs{First: &first}, false, "email")
			paginator := offset.New(pageArgs, totalCount)

			// Create fetcher and fetch
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return models.Users(mods...).Count(ctx, container.DB)
				},
				sqlboiler.OffsetToQueryMods,
			)

			fetchParams := paging.FetchParams{
				Offset:  paginator.Offset,
				Limit:   paginator.Limit,
				OrderBy: []paging.OrderBy{{Column: "email", Desc: false}},
			}
			users, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			Expect(users).To(HaveLen(5))
			// Verify sorted order (user1@, user10@, user11@, ...)
			Expect(users[0].Email).To(HaveSuffix("@example.com"))
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

			totalCount, err := models.Users().Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())
			Expect(totalCount).To(Equal(int64(100)))

			// Create fetcher once
			fetcher := sqlboiler.NewFetcher(
				func(ctx context.Context, mods ...qm.QueryMod) ([]*models.User, error) {
					return models.Users(mods...).All(ctx, container.DB)
				},
				func(ctx context.Context, mods ...qm.QueryMod) (int64, error) {
					return models.Users(mods...).Count(ctx, container.DB)
				},
				sqlboiler.OffsetToQueryMods,
			)

			// Paginate through all pages
			pageSize := 25
			pageArgs := &paging.PageArgs{
				First: &pageSize,
			}

			for page := 0; page < 4; page++ {
				paginator := offset.New(pageArgs, totalCount)

				fetchParams := paging.FetchParams{
					Offset:  paginator.Offset,
					Limit:   paginator.Limit,
					OrderBy: []paging.OrderBy{{Column: "created_at", Desc: true}},
				}
				users, err := fetcher.Fetch(ctx, fetchParams)
				Expect(err).ToNot(HaveOccurred())
				Expect(users).To(HaveLen(25))

				// Advance cursor for next page
				if page < 3 {
					nextCursor := offset.EncodeCursor((page + 1) * pageSize)
					pageArgs.After = nextCursor
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
			// Get count of published posts using SQLBoiler
			totalCount, err := models.Posts(qm.Where("published_at IS NOT NULL")).Count(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			first := 10
			pageArgs := &paging.PageArgs{First: &first}
			paginator := offset.New(pageArgs, totalCount)

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

			fetchParams := paging.FetchParams{
				Offset:  paginator.Offset,
				Limit:   paginator.Limit,
				OrderBy: []paging.OrderBy{{Column: "published_at", Desc: true}},
			}
			posts, err := fetcher.Fetch(ctx, fetchParams)
			Expect(err).ToNot(HaveOccurred())

			Expect(posts).To(HaveLen(10))
			// Verify all posts are published
			for _, post := range posts {
				Expect(post.PublishedAt).ToNot(BeNil())
			}

			hasNext, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNext).To(BeTrue()) // 25 users * 3 posts * 2/3 published = 50 posts
		})
	})
})
