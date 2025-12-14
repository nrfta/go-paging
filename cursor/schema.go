package cursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nrfta/go-paging"
)

// Direction represents the sort direction for a field.
type Direction bool

const (
	ASC  Direction = false
	DESC Direction = true
)

// fieldSpec defines a single sortable field in a schema.
// It contains the column name, cursor key, value extractor, and configuration.
type fieldSpec[T any] struct {
	name      string        // SQL column name: "posts.created_at"
	cursorKey string        // Short key for cursor: "c"
	extractor func(T) any   // Extract value from item
	isFixed   bool          // Fixed vs user-sortable
	direction *Direction    // For fixed fields (nil for user-sortable)
	position  int           // Declaration order
}

// Schema defines the sortable and fixed fields for cursor pagination.
// It enforces that cursor encoders and ORDER BY clauses match by providing
// a single source of truth for field configuration.
//
// Schema solves several critical issues:
// 1. Information leakage: Uses short cursor keys instead of column names
// 2. Encoder/OrderBy mismatch: Enforces they match by design
// 3. Dynamic sorting: Validates user sort choices and provides correct encoder
// 4. Fixed fields: Automatically includes tenant_id, id in ORDER BY
//
// Example:
//
//	var userSchema = cursor.NewSchema[*User]().
//	    FixedField("tenant_id", cursor.ASC, "t", func(u *User) any { return u.TenantID }).
//	    Field("name", "n", func(u *User) any { return u.Name }).
//	    Field("created_at", "c", func(u *User) any { return u.CreatedAt }).
//	    FixedField("id", cursor.DESC, "i", func(u *User) any { return u.ID })
type Schema[T any] struct {
	sortableFields map[string]*fieldSpec[T] // Map of column name to field spec
	fixedFields    []*fieldSpec[T]          // Fixed fields in declaration order
	allFields      []*fieldSpec[T]          // All fields in declaration order
	nextPosition   int                      // Track declaration order
}

// NewSchema creates a new Schema for cursor pagination.
func NewSchema[T any]() *Schema[T] {
	return &Schema[T]{
		sortableFields: make(map[string]*fieldSpec[T]),
		fixedFields:    make([]*fieldSpec[T], 0),
		allFields:      make([]*fieldSpec[T], 0),
		nextPosition:   0,
	}
}

// Field adds a user-sortable field to the schema.
// User-sortable fields can be specified in PageArgs.SortBy at runtime.
//
// Parameters:
//   - name: SQL column name (can be qualified: "posts.created_at")
//   - cursorKey: Short key for cursor encoding (e.g., "c")
//   - extractor: Function to extract the value from an item
//
// Example:
//
//	schema.Field("name", "n", func(u *User) any { return u.Name })
func (s *Schema[T]) Field(name, cursorKey string, extractor func(T) any) *Schema[T] {
	spec := &fieldSpec[T]{
		name:      name,
		cursorKey: cursorKey,
		extractor: extractor,
		isFixed:   false,
		direction: nil,
		position:  s.nextPosition,
	}
	s.nextPosition++

	s.sortableFields[name] = spec
	s.allFields = append(s.allFields, spec)

	return s
}

// FixedField adds a fixed field to the schema.
// Fixed fields are always included in ORDER BY and cursors but cannot be
// chosen by users at runtime.
//
// Parameters:
//   - name: SQL column name (can be qualified: "posts.id")
//   - direction: Sort direction (cursor.ASC or cursor.DESC)
//   - cursorKey: Short key for cursor encoding (e.g., "i")
//   - extractor: Function to extract the value from an item
//
// Declaration order matters:
//   - FixedField before Field: Prepended to ORDER BY (e.g., tenant_id for partitioning)
//   - FixedField after Field: Appended to ORDER BY (e.g., id for uniqueness)
//
// Example:
//
//	schema.FixedField("id", cursor.DESC, "i", func(u *User) any { return u.ID })
func (s *Schema[T]) FixedField(name string, direction Direction, cursorKey string, extractor func(T) any) *Schema[T] {
	spec := &fieldSpec[T]{
		name:      name,
		cursorKey: cursorKey,
		extractor: extractor,
		isFixed:   true,
		direction: &direction,
		position:  s.nextPosition,
	}
	s.nextPosition++

	s.fixedFields = append(s.fixedFields, spec)
	s.allFields = append(s.allFields, spec)

	return s
}

// EncoderFor validates the PageArgs and creates a Spec that implements CursorEncoder.
// It ensures that:
// 1. All sort fields in PageArgs.SortBy are valid (registered in schema)
// 2. The encoder extracts values for all sort fields + fixed fields
// 3. Short cursor keys are used (no information leakage)
//
// Returns an error if any sort field is invalid.
func (s *Schema[T]) EncoderFor(pageArgs PageArgs) (paging.CursorEncoder[T], error) {
	var sortFields []paging.Sort

	// Validate user's sort choices
	if pageArgs != nil && pageArgs.GetSortBy() != nil {
		sortFields = pageArgs.GetSortBy()

		for _, sort := range sortFields {
			if _, exists := s.sortableFields[sort.Column]; !exists {
				return nil, fmt.Errorf("invalid sort field: %s (not registered in schema)", sort.Column)
			}
		}
	}

	// Create Spec with validated sort fields
	return &Spec[T]{
		schema:      s,
		sortFields:  sortFields,
		fixedFields: s.fixedFields,
	}, nil
}

// BuildOrderBy constructs the complete ORDER BY clause including fixed fields.
// Fixed fields declared before user-sortable fields are prepended.
// Fixed fields declared after user-sortable fields are appended.
//
// Example:
//
//	schema.FixedField("tenant_id", ASC, ...)  // Declared first
//	schema.Field("name", ...)                  // User-sortable
//	schema.FixedField("id", DESC, ...)         // Declared last
//
//	BuildOrderBy([{Column: "name", Desc: true}])
//	// Returns: [tenant_id ASC, name DESC, id DESC]
func (s *Schema[T]) BuildOrderBy(userSorts []paging.Sort) []paging.Sort {
	result := make([]paging.Sort, 0)

	// Add fixed fields in declaration order, respecting their position relative to user-sortable fields
	// We need to:
	// 1. Add fixed fields that come before the first user-sortable field
	// 2. Add user sorts
	// 3. Add fixed fields that come after the last user-sortable field

	// Find position of first and last user-sortable field
	firstSortablePos := -1
	lastSortablePos := -1
	for _, spec := range s.allFields {
		if !spec.isFixed {
			if firstSortablePos == -1 {
				firstSortablePos = spec.position
			}
			lastSortablePos = spec.position
		}
	}

	// Special case: No user-sortable fields registered (only fixed fields)
	if firstSortablePos == -1 {
		for _, spec := range s.fixedFields {
			result = append(result, paging.Sort{
				Column: spec.name,
				Desc:   bool(*spec.direction),
			})
		}
		return result
	}

	// Add fixed fields that come before first sortable field (prepended)
	for _, spec := range s.fixedFields {
		if spec.position < firstSortablePos {
			result = append(result, paging.Sort{
				Column: spec.name,
				Desc:   bool(*spec.direction),
			})
		}
	}

	// Add user sorts
	result = append(result, userSorts...)

	// Add fixed fields that come after last sortable field (appended)
	for _, spec := range s.fixedFields {
		if spec.position > lastSortablePos {
			result = append(result, paging.Sort{
				Column: spec.name,
				Desc:   bool(*spec.direction),
			})
		}
	}

	return result
}

// Spec is the runtime configuration for cursor encoding/decoding.
// It implements CursorEncoder[T] and ensures encoder/OrderBy matching.
type Spec[T any] struct {
	schema      *Schema[T]
	sortFields  []paging.Sort     // User's chosen sorts
	fixedFields []*fieldSpec[T]   // Schema's fixed fields
}

// Encode implements CursorEncoder.Encode.
// It encodes the item using short cursor keys from the schema.
func (s *Spec[T]) Encode(item T) (*string, error) {
	values := make(map[string]any)

	// Extract values for user-chosen sort fields
	for _, sort := range s.sortFields {
		spec := s.schema.sortableFields[sort.Column]
		if spec != nil {
			values[spec.cursorKey] = spec.extractor(item)
		}
	}

	// Extract values for fixed fields
	for _, spec := range s.fixedFields {
		values[spec.cursorKey] = spec.extractor(item)
	}

	// Encode to JSON then base64
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return &encoded, nil
}

// Decode implements CursorEncoder.Decode.
// It decodes the cursor and maps short keys back to column names.
func (s *Spec[T]) Decode(cursor string) (*paging.CursorPosition, error) {
	// Decode from base64
	jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, errors.New("invalid cursor: not base64")
	}

	// Decode JSON
	var shortKeyValues map[string]any
	if err := json.Unmarshal(jsonBytes, &shortKeyValues); err != nil {
		return nil, errors.New("invalid cursor: not JSON")
	}

	// Map short keys back to column names
	values := make(map[string]any)

	// Build reverse mapping: cursorKey -> columnName
	keyToColumn := make(map[string]string)
	for _, spec := range s.schema.allFields {
		keyToColumn[spec.cursorKey] = spec.name
	}

	// Map short keys to column names
	for shortKey, value := range shortKeyValues {
		if columnName, exists := keyToColumn[shortKey]; exists {
			values[columnName] = value
		}
	}

	return &paging.CursorPosition{Values: values}, nil
}
