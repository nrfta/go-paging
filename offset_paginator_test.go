package paging_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging"
)

var _ = Describe("OffsetPaginator", func() {
	It("uses the default limit when no pageArgs.First is provided", func() {

		page := &paging.PageArgs{}

		paginator := paging.NewOffsetPaginator(page, 100)

		Expect(paginator.Limit).To(Equal(50))
		Expect(paginator.Offset).To(Equal(0))
	})

	It("parses the pageArgs correctly", func() {
		first := 10
		page := &paging.PageArgs{
			First: &first,
			After: paging.EncodeOffsetCursor(20),
		}

		paginator := paging.NewOffsetPaginator(page, 100)

		Expect(paginator.Limit).To(Equal(10))
		Expect(paginator.Offset).To(Equal(20))
	})

	It("creates a page info with provided info", func() {
		first := 10
		page := &paging.PageArgs{
			First: &first,
			After: paging.EncodeOffsetCursor(20),
		}

		paginator := paging.NewOffsetPaginator(page, 100)

		totalCount, _ := paginator.PageInfo.TotalCount()
		Expect(*totalCount).To(Equal(100))

		hasNextPage, _ := paginator.PageInfo.HasNextPage()
		Expect(hasNextPage).To(Equal(true))

		hasPreviousPage, _ := paginator.PageInfo.HasPreviousPage()
		Expect(hasPreviousPage).To(Equal(true))

		startCursor, _ := paginator.PageInfo.StartCursor()
		Expect(startCursor).To(Equal(paging.EncodeOffsetCursor(0)))

		endCursor, _ := paginator.PageInfo.EndCursor()
		Expect(endCursor).To(Equal(paging.EncodeOffsetCursor(90)))
	})

	It("returns the sqlboiler query mods", func() {
		first := 10
		page := &paging.PageArgs{
			First: &first,
			After: paging.EncodeOffsetCursor(20),
		}

		paginator := paging.NewOffsetPaginator(page, 100)

		mods := paginator.QueryMods()

		qm1 := reflect.TypeOf(mods[0]).String()
		Expect(qm1).To(Equal("qm.offsetQueryMod"))

		qm2 := reflect.TypeOf(mods[1]).String()
		Expect(qm2).To(Equal("qm.limitQueryMod"))

		qm3 := reflect.TypeOf(mods[2]).String()
		Expect(qm3).To(Equal("qm.orderByQueryMod"))
	})
})
