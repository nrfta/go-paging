package sqlboiler_test

import (
	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/sqlboiler"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OffsetToQueryMods", func() {
	Describe("Basic Functionality", func() {
		It("should return empty mods for empty params", func() {
			params := paging.FetchParams{}
			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(0))
		})

		It("should add OFFSET mod", func() {
			params := paging.FetchParams{
				Offset: 20,
			}
			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.offsetQueryMod"))
		})

		It("should add LIMIT mod", func() {
			params := paging.FetchParams{
				Limit: 10,
			}
			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.limitQueryMod"))
		})

		It("should add ORDER BY mod", func() {
			params := paging.FetchParams{
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}
			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.orderByQueryMod"))
		})

		It("should combine all mods together", func() {
			params := paging.FetchParams{
				Offset: 20,
				Limit:  10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(3))
			Expect(modTypeName(mods[0])).To(Equal("qm.offsetQueryMod"))
			Expect(modTypeName(mods[1])).To(Equal("qm.limitQueryMod"))
			Expect(modTypeName(mods[2])).To(Equal("qm.orderByQueryMod"))
		})
	})

	Describe("Edge Cases", func() {
		It("should skip OFFSET when offset is 0", func() {
			params := paging.FetchParams{
				Offset: 0,
				Limit:  10,
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.limitQueryMod"))
		})

		It("should skip LIMIT when limit is 0", func() {
			params := paging.FetchParams{
				Offset: 20,
				Limit:  0,
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(1))
			Expect(modTypeName(mods[0])).To(Equal("qm.offsetQueryMod"))
		})

		It("should handle empty OrderBy slice", func() {
			params := paging.FetchParams{
				Offset:  20,
				Limit:   10,
				OrderBy: []paging.OrderBy{},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			// Should only have OFFSET and LIMIT (no ORDER BY)
			Expect(mods).To(HaveLen(2))
		})

		It("should handle nil OrderBy", func() {
			params := paging.FetchParams{
				Offset:  20,
				Limit:   10,
				OrderBy: nil,
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			// Should only have OFFSET and LIMIT (no ORDER BY)
			Expect(mods).To(HaveLen(2))
		})
	})

	Describe("ORDER BY Clause Formatting", func() {
		It("should format single column DESC", func() {
			params := paging.FetchParams{
				Limit: 10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[1])).To(Equal("qm.orderByQueryMod"))
		})

		It("should format single column ASC (no DESC keyword)", func() {
			params := paging.FetchParams{
				Limit: 10,
				OrderBy: []paging.OrderBy{
					{Column: "name", Desc: false},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[1])).To(Equal("qm.orderByQueryMod"))
		})

		It("should format multiple columns with mixed directions", func() {
			params := paging.FetchParams{
				Limit: 10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "name", Desc: false},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[1])).To(Equal("qm.orderByQueryMod"))
		})

		It("should format three columns all DESC", func() {
			params := paging.FetchParams{
				Limit: 10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
					{Column: "updated_at", Desc: true},
					{Column: "id", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			Expect(mods).To(HaveLen(2))
			Expect(modTypeName(mods[1])).To(Equal("qm.orderByQueryMod"))
		})
	})

	Describe("Typical Pagination Scenarios", func() {
		It("should handle first page (offset=0)", func() {
			params := paging.FetchParams{
				Offset: 0,
				Limit:  10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			// First page: no OFFSET, only LIMIT and ORDER BY
			Expect(mods).To(HaveLen(2))
		})

		It("should handle second page (offset=10)", func() {
			params := paging.FetchParams{
				Offset: 10,
				Limit:  10,
				OrderBy: []paging.OrderBy{
					{Column: "created_at", Desc: true},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			// Second page: OFFSET, LIMIT, and ORDER BY
			Expect(mods).To(HaveLen(3))
		})

		It("should handle large offset (offset=1000)", func() {
			params := paging.FetchParams{
				Offset: 1000,
				Limit:  50,
				OrderBy: []paging.OrderBy{
					{Column: "id", Desc: false},
				},
			}

			mods := sqlboiler.OffsetToQueryMods(params)

			// Large offset: OFFSET, LIMIT, and ORDER BY
			Expect(mods).To(HaveLen(3))
		})
	})
})
