package paging_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging"
)

var _ = Describe("PageArgs", func() {
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

	It("should have zero values for basic PageArgs", func() {
		Expect(pa.SortByCols()).To(BeNil())
		Expect(pa.IsDesc()).To(BeFalse())
	})

	Describe("WithSortBy", func() {
		It("should handle a nil PageArgs arg", func() {
			pa := paging.WithSortBy(nil, true, "col1")
			Expect(pa).ToNot(BeNil())
		})

		Describe("Desc = true", func() {
			It("should set the PageArgs fields", func() {
				pa = paging.WithSortBy(pa, true, cols...)

				Expect(pa.IsDesc()).To(BeTrue())
				Expect(pa.SortByCols()).To(ContainElements(cols))
			})
		})

		Describe("Desc = false", func() {
			It("should set the PageArgs fields", func() {
				pa = paging.WithSortBy(pa, false, cols...)

				Expect(pa.IsDesc()).To(BeFalse())
				Expect(pa.SortByCols()).To(ContainElements(cols))
			})
		})

		Describe("Desc flag only", func() {
			It("should set isDesc to true", func() {
				pa = paging.WithSortBy(pa, true)

				Expect(pa.IsDesc()).To(BeTrue())
			})

			It("should set isDesc to false", func() {
				pa = paging.WithSortBy(pa, false)

				Expect(pa.IsDesc()).To(BeFalse())
			})
		})
	})
})
