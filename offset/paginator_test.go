package offset_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging"
	"github.com/nrfta/go-paging/offset"
)

var _ = Describe("Paginator", func() {
	Describe("Basic functionality", func() {
		It("uses the default limit when no pageArgs.First is provided", func() {
			page := &paging.PageArgs{}

			paginator := offset.New(page, 100)

			Expect(paginator.Limit).To(Equal(50))
			Expect(paginator.Offset).To(Equal(0))
		})

		It("parses the pageArgs correctly", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: offset.EncodeCursor(20),
			}

			paginator := offset.New(page, 100)

			Expect(paginator.Limit).To(Equal(10))
			Expect(paginator.Offset).To(Equal(20))
		})

		It("creates a page info with provided info", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: offset.EncodeCursor(20),
			}

			paginator := offset.New(page, 100)

			totalCount, _ := paginator.PageInfo.TotalCount()
			Expect(*totalCount).To(Equal(100))

			hasNextPage, _ := paginator.PageInfo.HasNextPage()
			Expect(hasNextPage).To(Equal(true))

			hasPreviousPage, _ := paginator.PageInfo.HasPreviousPage()
			Expect(hasPreviousPage).To(Equal(true))

			startCursor, _ := paginator.PageInfo.StartCursor()
			Expect(startCursor).To(Equal(offset.EncodeCursor(0)))

			endCursor, _ := paginator.PageInfo.EndCursor()
			Expect(endCursor).To(Equal(offset.EncodeCursor(90)))
		})

		It("returns the sqlboiler query mods", func() {
			first := 10
			page := &paging.PageArgs{
				First: &first,
				After: offset.EncodeCursor(20),
			}

			paginator := offset.New(page, 100)

			mods := paginator.QueryMods()

			qm1 := reflect.TypeOf(mods[0]).String()
			Expect(qm1).To(Equal("qm.offsetQueryMod"))

			qm2 := reflect.TypeOf(mods[1]).String()
			Expect(qm2).To(Equal("qm.limitQueryMod"))

			qm3 := reflect.TypeOf(mods[2]).String()
			Expect(qm3).To(Equal("qm.orderByQueryMod"))
		})
	})

	Describe("Order By", func() {
		var (
			pa   *paging.PageArgs
			cols []string
		)

		BeforeEach(func() {
			first := 0
			after := "after"
			pa = &paging.PageArgs{
				After: &after,
				First: &first,
			}
			cols = []string{"col1", "col2"}
		})

		Describe("Default", func() {
			It("should use `created_at` for default orderby column", func() {
				sut := offset.New(pa, 5)

				Expect(sut.GetOrderBy()).To(Equal("created_at"))
			})
		})

		Describe("Desc Flag & Cols", func() {
			Describe("Desc = true", func() {
				It("should set the Paginator orderBy field", func() {
					pa = paging.WithSortBy(pa, true, cols...)
					sut := offset.New(pa, 5)

					Expect(sut.GetOrderBy()).To(Equal("col1, col2 DESC"))
				})
			})

			Describe("Desc = false", func() {
				It("should set the Paginator orderBy field", func() {
					pa = paging.WithSortBy(pa, false, cols...)
					sut := offset.New(pa, 5)

					Expect(sut.GetOrderBy()).To(Equal("col1, col2"))
				})
			})
		})

		Describe("Desc Flag only", func() {
			Describe("Desc = true", func() {
				It("should set the Paginator orderBy field", func() {
					pa = paging.WithSortBy(pa, true)
					sut := offset.New(pa, 5)

					Expect(sut.GetOrderBy()).To(Equal("created_at DESC"))
				})
			})

			Describe("Desc = false", func() {
				It("should set the Paginator orderBy field", func() {
					pa = paging.WithSortBy(pa, false)
					sut := offset.New(pa, 5)

					Expect(sut.GetOrderBy()).To(Equal("created_at"))
				})
			})
		})
	})
})
