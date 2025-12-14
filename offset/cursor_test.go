package offset_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/paging-go/v2/offset"
)

var _ = Describe("Cursor Encoding/Decoding", func() {
	It("should be able to encode and decode the correct offset based cursor", func() {
		offsetValue := 34

		cursor := offset.EncodeCursor(offsetValue)
		Expect(*cursor).To(Equal("Y3Vyc29yOm9mZnNldDozNA=="))

		data := offset.DecodeCursor(cursor)
		Expect(data).To(Equal(offsetValue))
	})

	It("should handle nil cursor", func() {
		data := offset.DecodeCursor(nil)
		Expect(data).To(Equal(0))
	})

	It("should handle invalid cursor", func() {
		invalid := "invalid-cursor"
		data := offset.DecodeCursor(&invalid)
		Expect(data).To(Equal(0))
	})
})
