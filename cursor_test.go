package paging_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging"
)

var _ = Describe("Encode/Decode Offset Cursor", func() {
	It("should be able to encode and decode the correct offset based cursor`", func() {
		offset := 34

		cursor := paging.EncodeOffsetCursor(offset)
		Expect(*cursor).To(Equal("Y3Vyc29yOm9mZnNldDozNA=="))

		data := paging.DecodeOffsetCursor(cursor)
		Expect(data).To(Equal(offset))
	})
})
