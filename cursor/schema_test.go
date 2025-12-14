package cursor_test

import (
	"time"

	"github.com/nrfta/paging-go/v2"
	"github.com/nrfta/paging-go/v2/cursor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Schema", func() {
	var schema *cursor.Schema[*testUser]

	BeforeEach(func() {
		// Create schema with fixed fields before and after user-sortable fields
		schema = cursor.NewSchema[*testUser]().
			FixedField("tenant_id", cursor.ASC, "t", func(u *testUser) any {
				return u.TenantID
			}).
			Field("name", "n", func(u *testUser) any {
				return u.Name
			}).
			Field("email", "e", func(u *testUser) any {
				return u.Email
			}).
			Field("created_at", "c", func(u *testUser) any {
				return u.CreatedAt
			}).
			FixedField("id", cursor.DESC, "i", func(u *testUser) any {
				return u.ID
			})
	})

	Describe("EncoderFor", func() {
		It("should accept valid sort fields", func() {
			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "name", Desc: true},
				},
			}

			encoder, err := schema.EncoderFor(pageArgs)

			Expect(err).ToNot(HaveOccurred())
			Expect(encoder).ToNot(BeNil())
		})

		It("should accept multiple valid sort fields", func() {
			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "created_at", Desc: true},
					{Column: "name", Desc: false},
				},
			}

			encoder, err := schema.EncoderFor(pageArgs)

			Expect(err).ToNot(HaveOccurred())
			Expect(encoder).ToNot(BeNil())
		})

		It("should reject invalid sort fields", func() {
			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "invalid_field", Desc: true},
				},
			}

			encoder, err := schema.EncoderFor(pageArgs)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid sort field: invalid_field"))
			Expect(encoder).To(BeNil())
		})

		It("should work with nil PageArgs", func() {
			encoder, err := schema.EncoderFor(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(encoder).ToNot(BeNil())
		})

		It("should work with empty SortBy", func() {
			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{},
			}

			encoder, err := schema.EncoderFor(pageArgs)

			Expect(err).ToNot(HaveOccurred())
			Expect(encoder).ToNot(BeNil())
		})
	})

	Describe("BuildOrderBy", func() {
		It("should prepend fixed fields that come before user-sortable fields", func() {
			userSorts := []paging.Sort{
				{Column: "name", Desc: true},
			}

			orderBy := schema.BuildOrderBy(userSorts)

			Expect(orderBy).To(HaveLen(3))
			Expect(orderBy[0].Column).To(Equal("tenant_id"))
			Expect(orderBy[0].Desc).To(BeFalse()) // ASC
			Expect(orderBy[1].Column).To(Equal("name"))
			Expect(orderBy[1].Desc).To(BeTrue()) // DESC
			Expect(orderBy[2].Column).To(Equal("id"))
			Expect(orderBy[2].Desc).To(BeTrue()) // DESC
		})

		It("should append fixed fields that come after user-sortable fields", func() {
			userSorts := []paging.Sort{
				{Column: "email", Desc: false},
			}

			orderBy := schema.BuildOrderBy(userSorts)

			Expect(orderBy).To(HaveLen(3))
			Expect(orderBy[0].Column).To(Equal("tenant_id"))
			Expect(orderBy[1].Column).To(Equal("email"))
			Expect(orderBy[2].Column).To(Equal("id"))
		})

		It("should handle multiple user sorts", func() {
			userSorts := []paging.Sort{
				{Column: "created_at", Desc: true},
				{Column: "name", Desc: false},
			}

			orderBy := schema.BuildOrderBy(userSorts)

			Expect(orderBy).To(HaveLen(4))
			Expect(orderBy[0].Column).To(Equal("tenant_id"))
			Expect(orderBy[1].Column).To(Equal("created_at"))
			Expect(orderBy[2].Column).To(Equal("name"))
			Expect(orderBy[3].Column).To(Equal("id"))
		})

		It("should handle empty user sorts", func() {
			userSorts := []paging.Sort{}

			orderBy := schema.BuildOrderBy(userSorts)

			// Should only include fixed fields
			Expect(orderBy).To(HaveLen(2))
			Expect(orderBy[0].Column).To(Equal("tenant_id"))
			Expect(orderBy[1].Column).To(Equal("id"))
		})
	})

	Describe("Cursor Encoding/Decoding", func() {
		var (
			user    *testUser
			encoder paging.CursorEncoder[*testUser]
		)

		BeforeEach(func() {
			user = &testUser{
				ID:        "user-123",
				Name:      "Alice",
				Email:     "alice@example.com",
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				TenantID:  42,
			}

			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "name", Desc: true},
				},
			}

			var err error
			encoder, err = schema.EncoderFor(pageArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should use short cursor keys to prevent information leakage", func() {
			cursor, err := encoder.Encode(user)

			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).ToNot(BeNil())

			// Decode the cursor to inspect its contents
			decoded, err := encoder.Decode(*cursor)

			Expect(err).ToNot(HaveOccurred())
			Expect(decoded).ToNot(BeNil())

			// Verify cursor uses full column names internally
			Expect(decoded.Values).To(HaveKey("tenant_id"))
			Expect(decoded.Values).To(HaveKey("name"))
			Expect(decoded.Values).To(HaveKey("id"))

			// Verify it does NOT contain short keys
			Expect(decoded.Values).ToNot(HaveKey("t"))
			Expect(decoded.Values).ToNot(HaveKey("n"))
			Expect(decoded.Values).ToNot(HaveKey("i"))
		})

		It("should encode fixed fields in cursor", func() {
			cursor, err := encoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())

			decoded, err := encoder.Decode(*cursor)
			Expect(err).ToNot(HaveOccurred())

			// Fixed fields should be included
			Expect(decoded.Values["tenant_id"]).To(Equal(float64(42))) // JSON numbers are float64
			Expect(decoded.Values["id"]).To(Equal("user-123"))
		})

		It("should encode user-sortable fields in cursor", func() {
			cursor, err := encoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())

			decoded, err := encoder.Decode(*cursor)
			Expect(err).ToNot(HaveOccurred())

			// User sort field should be included
			Expect(decoded.Values["name"]).To(Equal("Alice"))
		})

		It("should handle invalid cursor format", func() {
			_, err := encoder.Decode("not-valid-base64!!!")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid cursor"))
		})
	})

	Describe("Dynamic Sorting", func() {
		It("should support changing sort field at runtime", func() {
			// First query: Sort by name
			pageArgs1 := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "name", Desc: true},
				},
			}

			encoder1, err := schema.EncoderFor(pageArgs1)
			Expect(err).ToNot(HaveOccurred())

			orderBy1 := schema.BuildOrderBy(pageArgs1.SortBy)
			Expect(orderBy1[1].Column).To(Equal("name"))

			// Second query: Sort by email
			pageArgs2 := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "email", Desc: false},
				},
			}

			encoder2, err := schema.EncoderFor(pageArgs2)
			Expect(err).ToNot(HaveOccurred())

			orderBy2 := schema.BuildOrderBy(pageArgs2.SortBy)
			Expect(orderBy2[1].Column).To(Equal("email"))

			// Encoders should be different
			Expect(encoder1).ToNot(Equal(encoder2))
		})
	})

	Describe("JOIN Query Support", func() {
		type userWithPost struct {
			UserID        string
			UserName      string
			PostID        string
			PostCreatedAt time.Time
		}

		It("should support qualified column names for JOINs", func() {
			joinSchema := cursor.NewSchema[*userWithPost]().
				Field("posts.created_at", "pc", func(uwp *userWithPost) any {
					return uwp.PostCreatedAt
				}).
				Field("users.name", "un", func(uwp *userWithPost) any {
					return uwp.UserName
				}).
				FixedField("posts.id", cursor.DESC, "pi", func(uwp *userWithPost) any {
					return uwp.PostID
				})

			pageArgs := &paging.PageArgs{
				SortBy: []paging.Sort{
					{Column: "users.name", Desc: false},
				},
			}

			encoder, err := joinSchema.EncoderFor(pageArgs)
			Expect(err).ToNot(HaveOccurred())

			orderBy := joinSchema.BuildOrderBy(pageArgs.SortBy)
			Expect(orderBy).To(HaveLen(2))
			Expect(orderBy[0].Column).To(Equal("users.name"))
			Expect(orderBy[1].Column).To(Equal("posts.id"))

			// Test encoding
			item := &userWithPost{
				UserID:        "user-1",
				UserName:      "Alice",
				PostID:        "post-1",
				PostCreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}

			cursor, err := encoder.Encode(item)
			Expect(err).ToNot(HaveOccurred())

			// Decode and verify qualified column names are preserved
			decoded, err := encoder.Decode(*cursor)
			Expect(err).ToNot(HaveOccurred())
			Expect(decoded.Values).To(HaveKey("users.name"))
			Expect(decoded.Values).To(HaveKey("posts.id"))

			// Short keys should NOT appear
			Expect(decoded.Values).ToNot(HaveKey("un"))
			Expect(decoded.Values).ToNot(HaveKey("pi"))
		})
	})

	Describe("Schema with Only Fixed Fields", func() {
		It("should work when all fields are fixed", func() {
			fixedOnlySchema := cursor.NewSchema[*testUser]().
				FixedField("tenant_id", cursor.ASC, "t", func(u *testUser) any {
					return u.TenantID
				}).
				FixedField("created_at", cursor.DESC, "c", func(u *testUser) any {
					return u.CreatedAt
				}).
				FixedField("id", cursor.DESC, "i", func(u *testUser) any {
					return u.ID
				})

			// No user sorts
			encoder, err := fixedOnlySchema.EncoderFor(nil)
			Expect(err).ToNot(HaveOccurred())

			orderBy := fixedOnlySchema.BuildOrderBy([]paging.Sort{})
			Expect(orderBy).To(HaveLen(3))
			Expect(orderBy[0].Column).To(Equal("tenant_id"))
			Expect(orderBy[1].Column).To(Equal("created_at"))
			Expect(orderBy[2].Column).To(Equal("id"))

			// Test encoding
			user := &testUser{
				ID:        "user-1",
				TenantID:  42,
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}

			cursor, err := encoder.Encode(user)
			Expect(err).ToNot(HaveOccurred())
			Expect(cursor).ToNot(BeNil())

			decoded, err := encoder.Decode(*cursor)
			Expect(err).ToNot(HaveOccurred())
			Expect(decoded.Values).To(HaveKey("tenant_id"))
			Expect(decoded.Values).To(HaveKey("created_at"))
			Expect(decoded.Values).To(HaveKey("id"))
		})
	})
})
