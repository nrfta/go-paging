// Package cursor provides high-performance cursor-based (keyset) pagination.
//
// Cursor pagination uses the values of sort columns to efficiently navigate
// large datasets without the performance degradation of offset pagination.
// It's ideal for infinite scroll, real-time feeds, and APIs with millions of records.
//
// Key Features:
//   - O(1) complexity regardless of page depth
//   - Consistent performance with proper indexes
//   - Stable results during concurrent writes
//   - Opaque cursor encoding (Base64 JSON)
//   - Composite key support for deterministic ordering
//
// Example usage:
//
//	encoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
//	    return map[string]any{
//	        "created_at": u.CreatedAt,
//	        "id":         u.ID,
//	    }
//	})
//
//	fetcher := sqlboiler.NewFetcher(..., sqlboiler.CursorToQueryMods)
//
//	users, _ := fetcher.Fetch(ctx, paging.FetchParams{...})
//	paginator := cursor.New(pageArgs, encoder, users)
//	conn, _ := cursor.BuildConnection(paginator, users, encoder, toDomainUser)
//
// Cursor Format:
//
//	Cursors are base64-encoded JSON objects containing column values:
//	{"created_at":"2024-01-01T00:00:00Z","id":"abc-123"}
//	→ eyJjcmVhdGVkX2F0IjoiMjAyNC0wMS0wMVQwMDowMDowMFoiLCJpZCI6ImFiYy0xMjMifQ==
//
// Performance:
//
//	Requires a composite index on sort columns:
//	CREATE INDEX idx ON table(col1 DESC, col2 DESC);
//
//	With proper indexing, all pages have similar performance (~5ms per page).
//
// Limitations:
//   - Forward pagination only (After + First). Backward pagination planned for Phase 2.5.
//   - Requires unique sort key (typically add ID as final column)
//   - PostgreSQL tuple comparison syntax (MySQL requires expanded form)
//   - Eventually consistent (cursors may become stale if data changes)
package cursor

import (
	"encoding/base64"
	"encoding/json"

	"github.com/nrfta/go-paging"
)

// CompositeCursorEncoder encodes multiple column values into an opaque cursor string.
// It implements the paging.CursorEncoder interface for composite key pagination.
//
// The encoder uses an extractor function to extract the relevant column values
// from each item, then encodes them as base64-encoded JSON.
//
// Type parameter T is the item type (e.g., *models.User).
type CompositeCursorEncoder[T any] struct {
	// extractor extracts the sort column values from an item.
	// The returned map should contain all columns used in ORDER BY.
	//
	// Example for sorting by (created_at DESC, id DESC):
	//   func(u *models.User) map[string]any {
	//       return map[string]any{
	//           "created_at": u.CreatedAt,
	//           "id":         u.ID,
	//       }
	//   }
	extractor func(T) map[string]any
}

// NewCompositeCursorEncoder creates a cursor encoder for composite key pagination.
//
// The extractor function should return a map of column names to their values
// for the columns used in sorting. These values will be encoded into the cursor
// and used to resume pagination.
//
// Example:
//
//	encoder := cursor.NewCompositeCursorEncoder(func(u *models.User) map[string]any {
//	    return map[string]any{
//	        "created_at": u.CreatedAt,
//	        "id":         u.ID,
//	    }
//	})
func NewCompositeCursorEncoder[T any](extractor func(T) map[string]any) paging.CursorEncoder[T] {
	return &CompositeCursorEncoder[T]{
		extractor: extractor,
	}
}

// Encode converts an item into an opaque cursor string.
// The cursor encodes the sort column values as base64-encoded JSON.
//
// Returns nil if the item has no values or if encoding fails.
func (e *CompositeCursorEncoder[T]) Encode(item T) (*string, error) {
	// Extract column values from the item
	values := e.extractor(item)
	if len(values) == 0 {
		return nil, nil
	}

	// Marshal values to JSON
	data, err := json.Marshal(values)
	if err != nil {
		return nil, nil // Gracefully return nil on error
	}

	// Base64 encode the JSON
	encoded := base64.URLEncoding.EncodeToString(data)
	return &encoded, nil
}

// Decode extracts cursor position from an opaque cursor string.
//
// The cursor is expected to be a base64-encoded JSON object containing
// column name/value pairs.
//
// Returns nil if the cursor is empty, invalid, or cannot be decoded.
// This graceful degradation ensures invalid cursors result in "start from beginning" behavior.
func (e *CompositeCursorEncoder[T]) Decode(cursor string) (*paging.CursorPosition, error) {
	// Handle empty cursor
	if cursor == "" {
		return nil, nil
	}

	// Base64 decode
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		// Gracefully return nil for invalid base64
		return nil, nil
	}

	// JSON unmarshal
	var values map[string]any
	if err := json.Unmarshal(decoded, &values); err != nil {
		// Gracefully return nil for invalid JSON
		return nil, nil
	}

	// Return nil if no values
	if len(values) == 0 {
		return nil, nil
	}

	return &paging.CursorPosition{
		Values: values,
	}, nil
}
