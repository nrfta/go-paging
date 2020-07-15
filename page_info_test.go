package paging_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging"
)

var _ = Describe("NewOffsetBasedPageInfo", func() {
	It("creates a page info with provided info", func() {
		size := 10

		pageInfo := paging.NewOffsetBasedPageInfo(&size, int64(100), 0)

		totalCount, _ := pageInfo.TotalCount()
		Expect(*totalCount).To(Equal(100))

		hasNextPage, _ := pageInfo.HasNextPage()
		Expect(hasNextPage).To(Equal(true))

		hasPreviousPage, _ := pageInfo.HasPreviousPage()
		Expect(hasPreviousPage).To(Equal(false))

		startCursor, _ := pageInfo.StartCursor()
		Expect(startCursor).To(Equal(paging.EncodeOffsetCursor(0)))

		endCursor, _ := pageInfo.EndCursor()
		Expect(endCursor).To(Equal(paging.EncodeOffsetCursor(90)))
	})

	It("hasNextPage works", func() {
		size := 10

		pageInfo := paging.NewOffsetBasedPageInfo(&size, int64(100), 90)

		hasNextPage, _ := pageInfo.HasNextPage()
		Expect(hasNextPage).To(Equal(false))
	})

	It("hasPreviousPage works", func() {
		size := 10

		pageInfo := paging.NewOffsetBasedPageInfo(&size, int64(100), 30)

		hasPreviousPage, _ := pageInfo.HasPreviousPage()
		Expect(hasPreviousPage).To(Equal(true))
	})

	It("endCursor works", func() {
		size := 10

		pageInfo := paging.NewOffsetBasedPageInfo(&size, int64(102), 100)

		endCursor, _ := pageInfo.EndCursor()
		Expect(endCursor).To(Equal(paging.EncodeOffsetCursor(100)))
	})
})

var _ = Describe("NewEmptyPageInfo", func() {
	It("creates a empty page info", func() {
		pageInfo := paging.NewEmptyPageInfo()

		totalCount, _ := pageInfo.TotalCount()
		Expect(totalCount).To(BeNil())

		hasNextPage, _ := pageInfo.HasNextPage()
		Expect(hasNextPage).To(Equal(false))

		hasPreviousPage, _ := pageInfo.HasPreviousPage()
		Expect(hasPreviousPage).To(Equal(false))

		startCursor, _ := pageInfo.StartCursor()
		Expect(startCursor).To(BeNil())

		endCursor, _ := pageInfo.EndCursor()
		Expect(endCursor).To(BeNil())
	})
})
