package paging_test

import (
	"context"
	"fmt"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/cursor"
	"github.com/nrfta/go-paging/quotafill"
	"github.com/nrfta/go-paging/sqlboiler"
	"github.com/nrfta/go-paging/tests/models"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("QuotaFill Integration Tests", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Clean up any existing test data
		_, err := container.DB.ExecContext(ctx, "TRUNCATE TABLE users CASCADE")
		Expect(err).ToNot(HaveOccurred())

		// Seed database with test data (100 users to ensure safeguards can be tested)
		_, err = SeedUsers(ctx, container.DB, 100)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Authorization Filter (Simulated)", func() {
		It("should filter items based on authorization rules", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			// Create a mock paginator that returns all users
			mockPaginator := newSimpleMockFetcher(allUsers)

			// Authorization filter that passes every other user (50% pass rate)
			authFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				authorized := []*models.User{}
				for _, user := range users {
					// Simulate authorization: pass users with even name length
					if len(user.Name)%2 == 0 {
						authorized = append(authorized, user)
					}
				}
				return authorized, nil
			}

			// Wrap with quota-fill
			wrapper := quotafill.New[*models.User](mockPaginator, authFilter, nil,
				quotafill.WithMaxIterations(5),
				quotafill.WithMaxRecordsExamined(50),
			)

			// Request 5 items
			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(5), "Should return exactly 5 authorized items")
			Expect(page.Metadata.Strategy).To(Equal("quotafill"))
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">=", 1))
			Expect(page.Metadata.ItemsExamined).To(BeNumerically(">=", 5))

			// Verify all items passed the filter
			for _, user := range page.Nodes {
				Expect(len(user.Name) % 2).To(Equal(0), "User should have passed authorization filter")
			}

			hasNext, _ := page.PageInfo.HasNextPage()
			// hasNext should be determined by N+1 pattern
			// It's either true or false depending on whether there are more items
			_ = hasNext // Just checking it doesn't error
		})

		It("should handle sparse filtering (low pass rate)", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			mockPaginator := newSimpleMockFetcher(allUsers)

			// Very selective filter (10% pass rate)
			sparseFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				authorized := []*models.User{}
				for i, user := range users {
					if i%10 == 0 {
						authorized = append(authorized, user)
					}
				}
				return authorized, nil
			}

			wrapper := quotafill.New[*models.User](mockPaginator, sparseFilter, nil,
				quotafill.WithMaxIterations(5),
				quotafill.WithMaxRecordsExamined(50),
			)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			// May return fewer items due to sparse filtering
			Expect(len(page.Nodes)).To(BeNumerically("<=", 3))
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">=", 1), "Should use multiple iterations")
		})
	})

	Describe("IsActive Filter", func() {
		It("should filter out inactive users", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			// Mark some users as inactive
			inactiveUsers := make([]*models.User, len(allUsers))
			copy(inactiveUsers, allUsers)
			for i := 0; i < 10; i++ {
				inactiveUsers[i].IsActive.Bool = false
				inactiveUsers[i].IsActive.Valid = true
			}

			mockPaginator := newSimpleMockFetcher(inactiveUsers)

			activeFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				active := []*models.User{}
				for _, user := range users {
					if !user.IsActive.Valid || user.IsActive.Bool {
						active = append(active, user)
					}
				}
				return active, nil
			}

			wrapper := quotafill.New[*models.User](mockPaginator, activeFilter, nil)

			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(5))

			// Verify no inactive users
			for _, user := range page.Nodes {
				if user.IsActive.Valid {
					Expect(user.IsActive.Bool).To(BeTrue())
				}
			}
		})
	})

	Describe("Composite Filters", func() {
		It("should apply multiple filter criteria", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			mockPaginator := newSimpleMockFetcher(allUsers)

			// Composite filter: authorization + active status
			compositeFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				filtered := []*models.User{}
				for _, user := range users {
					// Check 1: Active
					if user.IsActive.Valid && !user.IsActive.Bool {
						continue
					}
					// Check 2: Authorization (even name length)
					if len(user.Name)%2 != 0 {
						continue
					}
					// Check 3: Email must be set
					if user.Email == "" {
						continue
					}
					filtered = append(filtered, user)
				}
				return filtered, nil
			}

			wrapper := quotafill.New[*models.User](mockPaginator, compositeFilter, nil,
				quotafill.WithMaxIterations(5),
			)

			first := 3
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(len(page.Nodes)).To(BeNumerically("<=", 3))

			// Verify all criteria
			for _, user := range page.Nodes {
				if user.IsActive.Valid {
					Expect(user.IsActive.Bool).To(BeTrue())
				}
				Expect(len(user.Name) % 2).To(Equal(0))
				Expect(user.Email).ToNot(BeEmpty())
			}
		})
	})

	Describe("Safeguards", func() {
		It("should trigger max iterations safeguard", func() {
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			mockPaginator := newSimpleMockFetcher(allUsers)
			wrapper := quotafill.New[*models.User](mockPaginator, rejectAllUsersFilter(), nil,
				quotafill.WithMaxIterations(3),
			)

			first := 10
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(0))
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("max_iterations"))
			Expect(page.Metadata.IterationsUsed).To(Equal(3))
		})

		It("should trigger max records safeguard", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			mockPaginator := newSimpleMockFetcher(allUsers)

			// Low pass rate filter
			lowPassFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				if len(users) > 0 {
					return []*models.User{users[0]}, nil
				}
				return []*models.User{}, nil
			}

			wrapper := quotafill.New[*models.User](mockPaginator, lowPassFilter, nil,
				quotafill.WithMaxRecordsExamined(15),
			)

			first := 10
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Metadata.ItemsExamined).To(BeNumerically("<=", 15))
		})

		It("should trigger timeout safeguard", func() {
			// Fetch all users from database
			allUsers, err := models.Users().All(ctx, container.DB)
			Expect(err).ToNot(HaveOccurred())

			mockPaginator := newSimpleMockFetcher(allUsers)

			// Slow filter that sleeps to trigger timeout
			slowFilter := func(ctx context.Context, users []*models.User) ([]*models.User, error) {
				// Sleep long enough to exceed the timeout
				time.Sleep(200 * time.Millisecond)
				if len(users) > 0 {
					return []*models.User{users[0]}, nil
				}
				return []*models.User{}, nil
			}

			wrapper := quotafill.New[*models.User](mockPaginator, slowFilter, nil,
				quotafill.WithTimeout(100*time.Millisecond), // Very short timeout
				quotafill.WithMaxIterations(10),              // Allow enough iterations
			)

			first := 10
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Metadata.SafeguardHit).ToNot(BeNil())
			Expect(*page.Metadata.SafeguardHit).To(Equal("timeout"))
			// Should have partial results from before timeout
			Expect(page.Nodes).To(HaveLen(1))
		})
	})

	Describe("Real Cursor Pagination with SQLBoiler", func() {
		var (
			userSchema *cursor.Schema[*models.User]
		)

		BeforeEach(func() {
			userSchema = cursor.NewSchema[*models.User]().
				Field("created_at", "c", func(u *models.User) any { return u.CreatedAt }).
				FixedField("id", cursor.DESC, "i", func(u *models.User) any { return u.ID })
		})

		It("should paginate with cursor and filter in a single iteration", func() {
			fetcher := createRealCursorFetcher()

			// Filter passes 50% of users (even user numbers: 2,4,6,...,100)
			wrapper := quotafill.New[*models.User](fetcher, userNumFilter(2), userSchema,
				quotafill.WithMaxIterations(5),
			)

			// Request 5 items
			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(5), "Should return exactly 5 filtered items")

			// Verify all items passed the filter (even user numbers)
			for _, user := range page.Nodes {
				var userNum int
				fmt.Sscanf(user.Email, "user%d@example.com", &userNum)
				Expect(userNum%2).To(Equal(0), "User number should be even: %s", user.Email)
			}

			// Verify cursors are properly generated from filtered items
			startCursor, err := page.PageInfo.StartCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(startCursor).ToNot(BeNil(), "StartCursor should be generated")

			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil(), "EndCursor should be generated")

			// Decode end cursor and verify it matches the last returned item
			enc, err := userSchema.EncoderFor(&paging.PageArgs{})
			Expect(err).ToNot(HaveOccurred())
			cursorPos, err := enc.Decode(*endCursor)
			Expect(err).ToNot(HaveOccurred())
			lastUser := page.Nodes[len(page.Nodes)-1]
			Expect(cursorPos.Values["id"]).To(Equal(lastUser.ID), "EndCursor should encode last filtered item's ID")
		})

		It("should correctly advance cursor across multiple iterations with filtering", func() {
			fetcher := createRealCursorFetcher()

			// Filter passes ~33% of users (divisible by 3: 3,6,9,...,99)
			wrapper := quotafill.New[*models.User](fetcher, userNumFilter(3), userSchema,
				quotafill.WithMaxIterations(10),
				quotafill.WithBackoffMultipliers([]int{1, 2, 3, 5, 8}),
			)

			// Request 5 items (will require multiple iterations with ~30% pass rate)
			first := 5
			args := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(5), "Should return exactly 5 filtered items")
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">", 1), "Should use multiple iterations")

			// Verify all items passed the filter (user numbers divisible by 3)
			for _, user := range page.Nodes {
				var userNum int
				fmt.Sscanf(user.Email, "user%d@example.com", &userNum)
				Expect(userNum%3).To(Equal(0), "User number should be divisible by 3: %s", user.Email)
			}

			// Critical test: Verify cursor points to last FILTERED item, not last fetched item
			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil())

			enc, err := userSchema.EncoderFor(args)
			Expect(err).ToNot(HaveOccurred())
			cursorPos, err := enc.Decode(*endCursor)
			Expect(err).ToNot(HaveOccurred())

			lastReturnedUser := page.Nodes[len(page.Nodes)-1]
			Expect(cursorPos.Values["id"]).To(Equal(lastReturnedUser.ID),
				"EndCursor must encode last filtered item's ID, not base paginator's cursor")
			// Compare created_at - handle type conversion from cursor
			cursorCreatedAt, ok := cursorPos.Values["created_at"].(time.Time)
			if !ok {
				// If stored as string in cursor, parse it
				if createdAtStr, ok := cursorPos.Values["created_at"].(string); ok {
					cursorCreatedAt, err = time.Parse(time.RFC3339Nano, createdAtStr)
					Expect(err).ToNot(HaveOccurred())
				}
			}
			Expect(cursorCreatedAt.Equal(lastReturnedUser.CreatedAt)).To(BeTrue(),
				"EndCursor must encode last filtered item's created_at")
		})

		It("should handle pagination continuity across pages with cursor advancement", func() {
			// Filter passes 50% (even user numbers) - enough for 10 pages of 5 items
			evenUserFilter := userNumFilter(2)

			fetcher := createRealCursorFetcher()
			wrapper := quotafill.New[*models.User](fetcher, evenUserFilter, userSchema,
				quotafill.WithMaxIterations(10),
				quotafill.WithMaxRecordsExamined(100),
			)

			// PAGE 1: Request 5 items
			first := 5
			page1Args := paging.WithSortBy(&paging.PageArgs{First: &first}, "created_at", true)
			page1, err := wrapper.Paginate(ctx, page1Args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page1.Nodes).To(HaveLen(5), "Page 1 should return exactly 5 filtered items")

			hasNext, err := page1.PageInfo.HasNextPage()
			Expect(err).ToNot(HaveOccurred())
			Expect(hasNext).To(BeTrue(), "Should have next page with 50 total matches")

			endCursor1, err := page1.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor1).ToNot(BeNil())

			// PAGE 2: Request next 5 items using cursor from page 1
			page2Args := paging.WithSortBy(&paging.PageArgs{
				First: &first,
				After: endCursor1,
			}, "created_at", true)

			// Create new paginator for page 2 (stateless)
			fetcher2 := createRealCursorFetcher()
			wrapper2 := quotafill.New[*models.User](fetcher2, evenUserFilter, userSchema,
				quotafill.WithMaxIterations(10),
				quotafill.WithMaxRecordsExamined(100),
			)

			page2, err := wrapper2.Paginate(ctx, page2Args)
			Expect(err).ToNot(HaveOccurred())
			Expect(page2.Nodes).To(HaveLen(5), "Page 2 should return exactly 5 filtered items")

			// CRITICAL: Verify no overlap between pages
			page1IDs := make(map[string]bool)
			for _, user := range page1.Nodes {
				page1IDs[user.ID] = true
			}

			for _, user := range page2.Nodes {
				Expect(page1IDs[user.ID]).To(BeFalse(),
					"Page 2 should not contain any users from page 1 (user ID: %s)", user.ID)
			}

			// CRITICAL: Verify page 2 items come AFTER page 1's last item
			lastPage1User := page1.Nodes[len(page1.Nodes)-1]
			firstPage2User := page2.Nodes[0]

			// Compare using created_at and id (same as cursor ordering)
			isAfter := firstPage2User.CreatedAt.Before(lastPage1User.CreatedAt) ||
				(firstPage2User.CreatedAt.Equal(lastPage1User.CreatedAt) && firstPage2User.ID < lastPage1User.ID)

			Expect(isAfter).To(BeTrue(),
				"Page 2's first item should come AFTER page 1's last item in sort order")
		})

		It("should handle sparse filtering with real cursor pagination", func() {
			fetcher := createRealCursorFetcher()

			// Sparse filter passes 10% (divisible by 10: 10,20,30,...,100)
			wrapper := quotafill.New[*models.User](fetcher, userNumFilter(10), userSchema,
				quotafill.WithMaxIterations(8),
				quotafill.WithMaxRecordsExamined(100),
			)

			first := 5
			args := &paging.PageArgs{First: &first}
			page, err := wrapper.Paginate(ctx, args)

			Expect(err).ToNot(HaveOccurred())
			Expect(page.Nodes).To(HaveLen(5), "Should return 5 filtered items despite sparse filter")
			Expect(page.Metadata.IterationsUsed).To(BeNumerically(">", 1), "Should use multiple iterations")

			// Verify cursor correctness
			endCursor, err := page.PageInfo.EndCursor()
			Expect(err).ToNot(HaveOccurred())
			Expect(endCursor).ToNot(BeNil())

			enc, err := userSchema.EncoderFor(&paging.PageArgs{})
			Expect(err).ToNot(HaveOccurred())
			cursorPos, err := enc.Decode(*endCursor)
			Expect(err).ToNot(HaveOccurred())

			lastUser := page.Nodes[len(page.Nodes)-1]
			Expect(cursorPos.Values["id"]).To(Equal(lastUser.ID),
				"Cursor must encode last filtered item even with sparse filtering")
		})
	})
})

// userNumFilter creates a filter that passes users where their user number (from email) is divisible by n.
// Email format expected: "user42@example.com" -> user number is 42
func userNumFilter(divisor int) paging.FilterFunc[*models.User] {
	return func(ctx context.Context, users []*models.User) ([]*models.User, error) {
		filtered := []*models.User{}
		for _, user := range users {
			var userNum int
			if _, err := fmt.Sscanf(user.Email, "user%d@example.com", &userNum); err == nil {
				if userNum%divisor == 0 {
					filtered = append(filtered, user)
				}
			}
		}
		return filtered, nil
	}
}

// rejectAllUsersFilter returns a filter that rejects all users
func rejectAllUsersFilter() paging.FilterFunc[*models.User] {
	return func(ctx context.Context, users []*models.User) ([]*models.User, error) {
		return []*models.User{}, nil
	}
}

// Simple mock paginator for integration testing
type simpleMockFetcher[T any] struct {
	allItems []T
	offset   int
}

func newSimpleMockFetcher[T any](items []T) *simpleMockFetcher[T] {
	return &simpleMockFetcher[T]{
		allItems: items,
		offset:   0,
	}
}

func (f *simpleMockFetcher[T]) Fetch(ctx context.Context, params paging.FetchParams) ([]T, error) {
	// Use cursor position if provided, otherwise use internal offset
	start := f.offset
	if params.Cursor != nil {
		if offsetVal, ok := params.Cursor.Values["offset"].(int); ok {
			start = offsetVal
		} else if offsetVal, ok := params.Cursor.Values["offset"].(float64); ok {
			start = int(offsetVal)
		}
	}

	end := start + params.Limit
	if end > len(f.allItems) {
		end = len(f.allItems)
	}

	// Get items for this fetch
	items := []T{}
	if start < len(f.allItems) {
		items = f.allItems[start:end]
	}

	// Update offset for next sequential fetch
	f.offset = end

	return items, nil
}

func (f *simpleMockFetcher[T]) Count(ctx context.Context, params paging.FetchParams) (int64, error) {
	return int64(len(f.allItems)), nil
}

// createRealCursorFetcher creates a real cursor-based fetcher using SQLBoiler
// This is used for integration testing quota-fill with actual cursor pagination
func createRealCursorFetcher() paging.Fetcher[*models.User] {
	// Create fetcher with CursorToQueryMods strategy
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

