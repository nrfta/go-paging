package paging

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGoPaging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GoPaging Suite")
}

var _ = Describe("Order By clause", func() {
	var (
		pa   *PageArgs
		cols []string
	)

	BeforeEach(func() {
		first := 0
		after := "after"
		pa = &PageArgs{
			After: &after,
			First: &first,
		}
		cols = []string{"col1", "col2"}
	})

	Describe("WithSortBy", func() {
		It("should handle a nil PageArgs arg", func() {
			pa := WithSortBy(nil, true, "col1")
			Expect(pa).ToNot(BeNil())
		})
	})

	It("should have zero values for basic PageArgs", func() {
		Expect(pa.sortByCols).To(BeNil())
		Expect(pa.isDesc).To(BeFalse())
	})

	Describe("Default", func() {
		It("should use `created_at` for default orderby column", func() {
			sut := NewOffsetPaginator(pa, 5)

			Expect(sut.orderBy).To(Equal("created_at"))
		})
	})

	Describe("Desc Flag & Cols", func() {
		Describe("Desc = true", func() {
			It("should set the PageArgs fields", func() {
				pa = WithSortBy(pa, true, cols...)

				Expect(pa.isDesc).To(BeTrue())
				Expect(pa.sortByCols).To(ContainElements(cols))
			})

			It("should set the OffsetPaginator `orderBy` field", func() {
				pa = WithSortBy(pa, true, cols...)
				sut := NewOffsetPaginator(pa, 5)

				Expect(sut.orderBy).To(Equal("col1, col2 DESC"))
			})
		})

		Describe("Desc = false", func() {
			It("should set the PageArgs fields", func() {
				pa = WithSortBy(pa, false, cols...)

				Expect(pa.isDesc).To(BeFalse())
				Expect(pa.sortByCols).To(ContainElements(cols))
			})

			It("should set the OffsetPaginator `orderBy` field", func() {
				pa = WithSortBy(pa, false, cols...)
				sut := NewOffsetPaginator(pa, 5)

				Expect(sut.orderBy).To(Equal("col1, col2"))
			})
		})
	})

	Describe("Desc Flag only", func() {
		Describe("Desc = true", func() {
			It("should set the PageArgs fields", func() {
				pa = WithSortBy(pa, true)

				Expect(pa.isDesc).To(BeTrue())
			})

			It("should set the OffsetPaginator `orderBy` field", func() {
				pa = WithSortBy(pa, true)
				sut := NewOffsetPaginator(pa, 5)

				Expect(sut.orderBy).To(Equal("created_at DESC"))
			})
		})

		Describe("Desc = false", func() {
			It("should set the PageArgs fields", func() {
				pa = WithSortBy(pa, false)

				Expect(pa.isDesc).To(BeFalse())
			})

			It("should set the OffsetPaginator `orderBy` field", func() {
				pa = WithSortBy(pa, false)
				sut := NewOffsetPaginator(pa, 5)

				Expect(sut.orderBy).To(Equal("created_at"))
			})
		})
	})
})
