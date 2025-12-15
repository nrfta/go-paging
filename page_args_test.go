package paging_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/paging-go/v2"
)

var _ = Describe("PageArgs", func() {
	var pa *paging.PageArgs

	BeforeEach(func() {
		first := 10
		after := "cursor123"
		pa = &paging.PageArgs{
			First: &first,
			After: &after,
		}
	})

	It("should have zero values for basic PageArgs", func() {
		Expect(pa.GetSortBy()).To(BeNil())
	})

	Describe("WithSortBy", func() {
		It("should handle a nil PageArgs arg", func() {
			pa := paging.WithSortBy(nil, "created_at", true)
			Expect(pa).ToNot(BeNil())
			Expect(pa.GetSortBy()).To(HaveLen(1))
			Expect(pa.GetSortBy()[0].Column).To(Equal("created_at"))
			Expect(pa.GetSortBy()[0].Desc).To(BeTrue())
		})

		It("should set single sort field with DESC", func() {
			pa = paging.WithSortBy(pa, "name", true)

			Expect(pa.GetSortBy()).To(HaveLen(1))
			Expect(pa.GetSortBy()[0].Column).To(Equal("name"))
			Expect(pa.GetSortBy()[0].Desc).To(BeTrue())
		})

		It("should set single sort field with ASC", func() {
			pa = paging.WithSortBy(pa, "email", false)

			Expect(pa.GetSortBy()).To(HaveLen(1))
			Expect(pa.GetSortBy()[0].Column).To(Equal("email"))
			Expect(pa.GetSortBy()[0].Desc).To(BeFalse())
		})
	})

	Describe("WithMultiSort", func() {
		It("should handle a nil PageArgs arg", func() {
			pa := paging.WithMultiSort(nil,
				paging.Sort{Column: "created_at", Desc: true},
				paging.Sort{Column: "id", Desc: false},
			)
			Expect(pa).ToNot(BeNil())
			Expect(pa.GetSortBy()).To(HaveLen(2))
		})

		It("should set multiple sort fields with different directions", func() {
			pa = paging.WithMultiSort(pa,
				paging.Sort{Column: "created_at", Desc: true},
				paging.Sort{Column: "name", Desc: false},
				paging.Sort{Column: "id", Desc: true},
			)

			Expect(pa.GetSortBy()).To(HaveLen(3))
			Expect(pa.GetSortBy()[0]).To(Equal(paging.Sort{Column: "created_at", Desc: true}))
			Expect(pa.GetSortBy()[1]).To(Equal(paging.Sort{Column: "name", Desc: false}))
			Expect(pa.GetSortBy()[2]).To(Equal(paging.Sort{Column: "id", Desc: true}))
		})
	})
})

var _ = Describe("NewEmptyPageInfo", func() {
	It("should return empty PageInfo with nil/false values", func() {
		pageInfo := paging.NewEmptyPageInfo()

		totalCount, err := pageInfo.TotalCount()
		Expect(err).ToNot(HaveOccurred())
		Expect(totalCount).To(BeNil())

		startCursor, err := pageInfo.StartCursor()
		Expect(err).ToNot(HaveOccurred())
		Expect(startCursor).To(BeNil())

		endCursor, err := pageInfo.EndCursor()
		Expect(err).ToNot(HaveOccurred())
		Expect(endCursor).To(BeNil())

		hasNext, err := pageInfo.HasNextPage()
		Expect(err).ToNot(HaveOccurred())
		Expect(hasNext).To(BeFalse())

		hasPrev, err := pageInfo.HasPreviousPage()
		Expect(err).ToNot(HaveOccurred())
		Expect(hasPrev).To(BeFalse())
	})
})
