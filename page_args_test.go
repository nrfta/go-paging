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

var _ = Describe("ValidatePageSize", func() {
	It("should accept nil args", func() {
		err := paging.ValidatePageSize(nil, 100)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should accept args with nil First", func() {
		args := &paging.PageArgs{}
		err := paging.ValidatePageSize(args, 100)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should accept page size within limit", func() {
		first := 50
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 100)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should accept page size equal to limit", func() {
		first := 100
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 100)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should reject page size exceeding limit", func() {
		first := 1010
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 1000)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("1010"))
		Expect(err.Error()).To(ContainSubstring("1000"))
		Expect(err.Error()).To(ContainSubstring("exceeds maximum"))
	})

	It("should return PageSizeError with correct values", func() {
		first := 150
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 100)
		Expect(err).To(HaveOccurred())

		var pageSizeErr *paging.PageSizeError
		Expect(err).To(BeAssignableToTypeOf(pageSizeErr))

		pageSizeErr = err.(*paging.PageSizeError)
		Expect(pageSizeErr.Requested).To(Equal(150))
		Expect(pageSizeErr.Maximum).To(Equal(100))
	})

	It("should handle very large page size requests", func() {
		first := 999999
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 100)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("999999"))
	})

	It("should use DefaultMaxPageSize when maxPageSize is 0", func() {
		first := 1500
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 0) // Should use DefaultMaxPageSize (1000)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("1500"))
		Expect(err.Error()).To(ContainSubstring("1000"))
	})

	It("should accept page size within DefaultMaxPageSize when using 0", func() {
		first := 500
		args := &paging.PageArgs{First: &first}
		err := paging.ValidatePageSize(args, 0) // Should use DefaultMaxPageSize (1000)
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("PageArgs.Validate", func() {
	It("should validate using DefaultMaxPageSize", func() {
		first := 500
		args := &paging.PageArgs{First: &first}
		err := args.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should reject page size exceeding DefaultMaxPageSize", func() {
		first := 1500
		args := &paging.PageArgs{First: &first}
		err := args.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("1500"))
		Expect(err.Error()).To(ContainSubstring("1000"))
	})

	It("should accept nil args", func() {
		var args *paging.PageArgs
		err := args.Validate()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should accept page size equal to DefaultMaxPageSize", func() {
		first := 1000
		args := &paging.PageArgs{First: &first}
		err := args.Validate()
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("PageConfig", func() {
	Describe("NewPageConfig", func() {
		It("should create config with default values", func() {
			config := paging.NewPageConfig()
			Expect(config.DefaultSize).To(Equal(paging.DefaultPageSize))
			Expect(config.MaxSize).To(Equal(paging.DefaultMaxPageSize))
		})
	})

	Describe("WithDefaultSize", func() {
		It("should set default size", func() {
			config := paging.NewPageConfig().WithDefaultSize(25)
			Expect(config.DefaultSize).To(Equal(25))
		})

		It("should ignore zero or negative values", func() {
			config := paging.NewPageConfig().WithDefaultSize(0)
			Expect(config.DefaultSize).To(Equal(paging.DefaultPageSize))

			config = paging.NewPageConfig().WithDefaultSize(-10)
			Expect(config.DefaultSize).To(Equal(paging.DefaultPageSize))
		})

		It("should support method chaining", func() {
			config := paging.NewPageConfig().
				WithDefaultSize(25).
				WithMaxSize(500)
			Expect(config.DefaultSize).To(Equal(25))
			Expect(config.MaxSize).To(Equal(500))
		})
	})

	Describe("WithMaxSize", func() {
		It("should set max size", func() {
			config := paging.NewPageConfig().WithMaxSize(500)
			Expect(config.MaxSize).To(Equal(500))
		})

		It("should ignore zero or negative values", func() {
			config := paging.NewPageConfig().WithMaxSize(0)
			Expect(config.MaxSize).To(Equal(paging.DefaultMaxPageSize))

			config = paging.NewPageConfig().WithMaxSize(-10)
			Expect(config.MaxSize).To(Equal(paging.DefaultMaxPageSize))
		})
	})

	Describe("EffectiveLimit", func() {
		It("should return default size when args is nil", func() {
			config := paging.NewPageConfig().WithDefaultSize(25)
			limit := config.EffectiveLimit(nil)
			Expect(limit).To(Equal(25))
		})

		It("should return default size when First is nil", func() {
			config := paging.NewPageConfig().WithDefaultSize(25)
			args := &paging.PageArgs{}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(25))
		})

		It("should return default size when First is zero", func() {
			config := paging.NewPageConfig().WithDefaultSize(25)
			zero := 0
			args := &paging.PageArgs{First: &zero}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(25))
		})

		It("should return default size when First is negative", func() {
			config := paging.NewPageConfig().WithDefaultSize(25)
			negative := -10
			args := &paging.PageArgs{First: &negative}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(25))
		})

		It("should return First when within limits", func() {
			config := paging.NewPageConfig().WithMaxSize(100)
			first := 50
			args := &paging.PageArgs{First: &first}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(50))
		})

		It("should cap First to MaxSize when exceeded", func() {
			config := paging.NewPageConfig().WithMaxSize(100)
			first := 500
			args := &paging.PageArgs{First: &first}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(100))
		})

		It("should handle nil config gracefully", func() {
			var config *paging.PageConfig
			first := 50
			args := &paging.PageArgs{First: &first}
			limit := config.EffectiveLimit(args)
			Expect(limit).To(Equal(50))
		})

		It("should use system defaults when config values are zero", func() {
			config := &paging.PageConfig{DefaultSize: 0, MaxSize: 0}
			limit := config.EffectiveLimit(nil)
			Expect(limit).To(Equal(paging.DefaultPageSize))
		})
	})

	Describe("Validate", func() {
		It("should accept valid page size", func() {
			config := paging.NewPageConfig().WithMaxSize(100)
			first := 50
			args := &paging.PageArgs{First: &first}
			err := config.Validate(args)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should accept page size equal to max", func() {
			config := paging.NewPageConfig().WithMaxSize(100)
			first := 100
			args := &paging.PageArgs{First: &first}
			err := config.Validate(args)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject page size exceeding max", func() {
			config := paging.NewPageConfig().WithMaxSize(100)
			first := 150
			args := &paging.PageArgs{First: &first}
			err := config.Validate(args)
			Expect(err).To(HaveOccurred())

			var pageSizeErr *paging.PageSizeError
			Expect(err).To(BeAssignableToTypeOf(pageSizeErr))
			pageSizeErr = err.(*paging.PageSizeError)
			Expect(pageSizeErr.Requested).To(Equal(150))
			Expect(pageSizeErr.Maximum).To(Equal(100))
		})

		It("should accept nil args", func() {
			config := paging.NewPageConfig()
			err := config.Validate(nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should accept nil First", func() {
			config := paging.NewPageConfig()
			args := &paging.PageArgs{}
			err := config.Validate(args)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle nil config gracefully", func() {
			var config *paging.PageConfig
			first := 500
			args := &paging.PageArgs{First: &first}
			err := config.Validate(args)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("PageArgs.ValidateWith", func() {
	It("should validate using custom config", func() {
		config := paging.NewPageConfig().WithMaxSize(100)
		first := 50
		args := &paging.PageArgs{First: &first}
		err := args.ValidateWith(config)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should reject page size exceeding custom max", func() {
		config := paging.NewPageConfig().WithMaxSize(100)
		first := 150
		args := &paging.PageArgs{First: &first}
		err := args.ValidateWith(config)
		Expect(err).To(HaveOccurred())
	})
})
