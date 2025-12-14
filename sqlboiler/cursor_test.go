package sqlboiler_test

import (
	"time"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/sqlboiler"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CursorToQueryMods", func() {
	Describe("Basic Functionality", func() {
		It("should return empty mods for empty params", func() {
			params := paging.FetchParams{}
			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(0))
		})

		It("should add LIMIT mod", func() {
			params := paging.FetchParams{
				Limit: 10,
			}
			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.limitQueryMod"))
		})

		It("should add ORDER BY mod", func() {
			params := paging.FetchParams{
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.orderByQueryMod"))
		})

		It("should add WHERE mod with cursor", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					"id":         "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[0])).To(whereModMatcher())
		})

		It("should combine all mods together", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					"id":         "user-123",
				},
			}

			params := paging.FetchParams{
				Limit:  10,
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(3))
			Expect(modTypeName(mods[0])).To(whereModMatcher())
			Expect(modTypeName(mods[1])).To(Equal("qm.limitQueryMod"))
			Expect(modTypeName(mods[2])).To(Equal("qm.orderByQueryMod"))
		})
	})

	Describe("Graceful Handling", func() {
		It("should handle nil cursor gracefully", func() {
			params := paging.FetchParams{
				Cursor: nil,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.orderByQueryMod"))
		})

		It("should handle empty cursor values gracefully", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should only have ORDER BY (no WHERE)
			Expect(mods).To(HaveLen(1))
		})

		It("should handle missing cursor column gracefully", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					// Missing "id" column
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true}, // This column is missing in cursor
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should only have ORDER BY (no WHERE due to missing column)
			Expect(mods).To(HaveLen(1))
		})
	})
})

var _ = Describe("KeysetWhereClause", func() {
	Describe("WHERE Clause Generation", func() {
		It("should generate correct WHERE clause for DESC order", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					"id":         "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(modTypeName(mods[0])).To(whereModMatcher())
		})

		It("should generate correct WHERE clause for ASC order", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"name": "John",
					"id":   "user-456",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "name", Desc: false},
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[0])).To(whereModMatcher())
		})

		It("should handle single column", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"id": "user-789",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[0])).To(whereModMatcher())
		})

		It("should handle three columns", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					"name":       "John",
					"id":         "user-999",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "name", Desc: false},
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[0])).To(whereModMatcher())
		})
	})
})

var _ = Describe("ConvertValueForSQL", func() {
	Describe("Value Type Handling", func() {
		It("should handle time.Time values", func() {
			timestamp := time.Date(2024, 1, 1, 12, 30, 45, 0, time.UTC)

			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": timestamp,
					"id":         "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should successfully create WHERE mod with time value
			Expect(mods).To(HaveLen(2))
		})

		It("should handle RFC3339 string values", func() {
			rfcString := "2024-01-01T12:30:45Z"

			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"created_at": rfcString,
					"id":         "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should successfully create WHERE mod
			Expect(mods).To(HaveLen(2))
		})

		It("should handle integer values", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"age": 25,
					"id":  "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "age", Desc: false},
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should successfully create WHERE mod
			Expect(mods).To(HaveLen(2))
		})

		It("should handle float64 values from JSON", func() {
			cursor := &paging.CursorPosition{
				Values: map[string]any{
					"score": 42.5,
					"id":    "user-123",
				},
			}

			params := paging.FetchParams{
				Cursor: cursor,
				OrderBy: []paging.Sort{
					{Column: "score", Desc: true},
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.CursorToQueryMods(params)

			// Should successfully create WHERE mod
			Expect(mods).To(HaveLen(2))
		})
	})
})
