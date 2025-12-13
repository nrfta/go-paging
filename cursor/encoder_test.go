package cursor_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/nrfta/go-paging/cursor"
)

// testUser is a simple test struct for cursor encoding
type testUser struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
	TenantID  int
	Age       int
}

var _ = Describe("Cursor Encoding/Decoding", func() {
	var encoder *cursor.CompositeCursorEncoder[*testUser]

	BeforeEach(func() {
		// Create encoder that extracts created_at and id
		encoder = cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
			return map[string]any{
				"created_at": u.CreatedAt,
				"id":         u.ID,
			}
		}).(*cursor.CompositeCursorEncoder[*testUser])
	})

	Describe("Encode", func() {
		It("should encode a single item with multiple columns", func() {
			user := &testUser{
				ID:        "abc-123",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}

			cursor, err := encoder.Encode(user)

			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).ToNot(BeNil())
			Expect(*cursor).ToNot(BeEmpty())
		})

		It("should encode different data types correctly", func() {
			user := &testUser{
				ID:        "uuid-456",
				CreatedAt: time.Date(2024, 6, 15, 12, 30, 45, 0, time.UTC),
			}

			cursor, err := encoder.Encode(user)

			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).ToNot(BeNil())
		})

		It("should handle items with integer values", func() {
			// Create encoder that includes age (int)
			intEncoder := cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
				return map[string]any{
					"age": u.Age,
					"id":  u.ID,
				}
			})

			user := &testUser{
				ID:  "test-id",
				Age: 25,
			}

			cursor, err := intEncoder.Encode(user)

			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).ToNot(BeNil())
		})

		It("should return nil for empty extractor result", func() {
			// Create encoder that returns empty map
			emptyEncoder := cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
				return map[string]any{}
			})

			user := &testUser{ID: "test"}

			cursor, err := emptyEncoder.Encode(user)

			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).To(BeNil())
		})
	})

	Describe("Decode", func() {
		It("should decode a valid cursor", func() {
			user := &testUser{
				ID:        "abc-123",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}

			// Encode first
			encodedCursor, err := encoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())
			Expect(encodedCursor).ToNot(BeNil())

			// Decode
			pos, err := encoder.Decode(*encodedCursor)

			Expect(err).ToNot(HaveOccurred())
			Expect(pos).ToNot(BeNil())
			Expect(pos.Values).To(HaveLen(2))
			Expect(pos.Values).To(HaveKey("id"))
			Expect(pos.Values).To(HaveKey("created_at"))
			Expect(pos.Values["id"]).To(Equal("abc-123"))
		})

		It("should handle empty cursor string", func() {
			pos, err := encoder.Decode("")

			Expect(err).ToNot(HaveOccurred())
			Expect(pos).To(BeNil())
		})

		It("should handle invalid base64", func() {
			pos, err := encoder.Decode("invalid-base64!!!")

			Expect(err).ToNot(HaveOccurred())
			Expect(pos).To(BeNil())
		})

		It("should handle invalid JSON", func() {
			// Base64 encode invalid JSON
			invalidJSON := "e25vdCB2YWxpZCBqc29ufQ==" // base64("{not valid json}")

			pos, err := encoder.Decode(invalidJSON)

			Expect(err).ToNot(HaveOccurred())
			Expect(pos).To(BeNil())
		})

		It("should handle malformed cursor gracefully", func() {
			malformedCursors := []string{
				"",
				"abc",
				"!!!",
				"AAA===",
			}

			for _, malformed := range malformedCursors {
				pos, err := encoder.Decode(malformed)
				Expect(err).ToNot(HaveOccurred())
				Expect(pos).To(BeNil())
			}
		})
	})

	Describe("Round-trip encoding", func() {
		It("should successfully encode and decode with string values", func() {
			user := &testUser{
				ID:        "user-789",
				CreatedAt: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
			}

			// Encode
			encodedCursor, err := encoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())
			Expect(encodedCursor).ToNot(BeNil())

			// Decode
			pos, err := encoder.Decode(*encodedCursor)
			Expect(err).ToNot(HaveOccurred())
			Expect(pos).ToNot(BeNil())

			// Verify values
			Expect(pos.Values["id"]).To(Equal("user-789"))
			Expect(pos.Values["created_at"]).ToNot(BeNil())
		})

		It("should preserve integer values through encode/decode", func() {
			intEncoder := cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
				return map[string]any{
					"age": u.Age,
					"id":  u.ID,
				}
			})

			user := &testUser{
				ID:  "test-id",
				Age: 42,
			}

			// Encode
			encodedCursor, err := intEncoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())

			// Decode
			pos, err := intEncoder.Decode(*encodedCursor)
			Expect(err).ToNot(HaveOccurred())

			// Verify age is preserved (JSON numbers decode as float64)
			Expect(pos.Values["age"]).To(BeNumerically("==", 42))
		})

		It("should handle multiple users with different timestamps", func() {
			users := []*testUser{
				{ID: "user-1", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: "user-2", CreatedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				{ID: "user-3", CreatedAt: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			}

			for _, user := range users {
				// Encode
				encodedCursor, err := encoder.Encode(user)
				Expect(err).ToNot(HaveOccurred())

				// Decode
				pos, err := encoder.Decode(*encodedCursor)
				Expect(err).ToNot(HaveOccurred())

				// Verify ID is preserved
				Expect(pos.Values["id"]).To(Equal(user.ID))
			}
		})
	})

	Describe("Multiple column encoding", func() {
		It("should encode three columns correctly", func() {
			threeColEncoder := cursor.NewCompositeCursorEncoder(func(u *testUser) map[string]any {
				return map[string]any{
					"created_at": u.CreatedAt,
					"age":        u.Age,
					"id":         u.ID,
				}
			})

			user := &testUser{
				ID:        "multi-col-user",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Age:       30,
			}

			// Encode
			encodedCursor, err := threeColEncoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())

			// Decode
			pos, err := threeColEncoder.Decode(*encodedCursor)
			Expect(err).ToNot(HaveOccurred())

			// Verify all three columns
			Expect(pos.Values).To(HaveLen(3))
			Expect(pos.Values).To(HaveKey("created_at"))
			Expect(pos.Values).To(HaveKey("age"))
			Expect(pos.Values).To(HaveKey("id"))
		})
	})
})
