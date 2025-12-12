package sqlboiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/aarondl/sqlboiler/v4/queries"
	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
)

// CursorToQueryMods converts FetchParams into SQLBoiler query mods for cursor-based pagination.
// This is the strategy-specific query builder for keyset pagination.
//
// The conversion follows these rules:
//   - Cursor → qm.Where("(col1, col2) OP (?, ?)", val1, val2) using tuple comparison
//   - Limit → qm.Limit(n)
//   - OrderBy → qm.OrderBy("col1 DESC, col2 DESC")
//
// This function is used by cursor.Paginator when creating a SQLBoiler fetcher.
//
// Example:
//
//	fetcher := sqlboiler.NewFetcher(
//	    queryFunc,
//	    countFunc,
//	    sqlboiler.CursorToQueryMods, // ← Use cursor strategy
//	)
//
// Requirements:
//   - PostgreSQL database (for tuple comparison syntax)
//   - Composite index on sort columns: CREATE INDEX idx ON table(col1 DESC, col2 DESC)
func CursorToQueryMods(params paging.FetchParams) []qm.QueryMod {
	mods := []qm.QueryMod{}

	// Add WHERE clause for cursor position (keyset comparison)
	if params.Cursor != nil && len(params.OrderBy) > 0 {
		whereClause, args := buildKeysetWhereClause(params.Cursor, params.OrderBy)
		if whereClause != "" {
			// Use raw SQL query mod to inject WHERE clause directly
			// This bypasses qm.Where's tuple comparison limitations
			mods = append(mods, rawWhereClause(whereClause, args))
		}
	}

	// Add LIMIT
	if params.Limit > 0 {
		mods = append(mods, qm.Limit(params.Limit))
	}

	// Add ORDER BY
	if len(params.OrderBy) > 0 {
		mods = append(mods, qm.OrderBy(buildOrderByClause(params.OrderBy)))
	}

	return mods
}

// buildKeysetWhereClause builds a WHERE clause for keyset pagination using expanded comparison.
//
// Uses expanded comparison form which is compatible with SQLBoiler:
//
//	DESC order: col1 < ? OR (col1 = ? AND col2 < ?)
//	ASC order:  col1 > ? OR (col1 = ? AND col2 > ?)
//
// The operator is determined by the sort direction:
//   - DESC: use < (get records BEFORE cursor)
//   - ASC: use > (get records AFTER cursor)
//
// Returns empty string if cursor is invalid or missing required columns.
func buildKeysetWhereClause(cursor *paging.CursorPosition, orderBy []paging.OrderBy) (string, []interface{}) {
	if cursor == nil || len(cursor.Values) == 0 || len(orderBy) == 0 {
		return "", nil
	}

	// Determine comparison operator based on sort direction
	operator := ">"
	if orderBy[0].Desc {
		operator = "<"
	}

	// Build expanded comparison: col1 OP ? OR (col1 = ? AND col2 OP ?)
	var parts []string
	var args []interface{}

	for i, order := range orderBy {
		// Get value from cursor
		val, exists := cursor.Values[order.Column]
		if !exists {
			// If cursor doesn't have this column, skip WHERE clause
			return "", nil
		}

		if i == 0 {
			// First column: simple comparison
			parts = append(parts, fmt.Sprintf("%s %s ?", order.Column, operator))
			args = append(args, convertValueForSQL(val))
		} else {
			// Build equality checks for all previous columns
			var equalityParts []string
			for j := 0; j < i; j++ {
				prevOrder := orderBy[j]
				prevVal, _ := cursor.Values[prevOrder.Column]
				equalityParts = append(equalityParts, fmt.Sprintf("%s = ?", prevOrder.Column))
				args = append(args, convertValueForSQL(prevVal))
			}

			// Add comparison for current column
			part := fmt.Sprintf("(%s AND %s %s ?)",
				strings.Join(equalityParts, " AND "),
				order.Column,
				operator,
			)
			parts = append(parts, part)
			args = append(args, convertValueForSQL(val))
		}
	}

	// Join with OR and wrap in parentheses
	whereClause := "(" + strings.Join(parts, " OR ") + ")"

	return whereClause, args
}

// rawWhereClause creates a custom query mod that injects a WHERE clause directly.
// This is necessary because qm.Where doesn't properly handle tuple comparisons.
//
// The function creates a query mod that:
//  1. Adds the WHERE clause to the query's WHERE buffer
//  2. Appends the arguments to the query's argument list
//
// This approach bypasses SQLBoiler's WHERE clause processing and injects
// the clause directly into the final SQL, which allows tuple comparisons to work.
func rawWhereClause(clause string, args []interface{}) qm.QueryMod {
	return qm.QueryModFunc(func(q *queries.Query) {
		queries.AppendWhere(q, clause, args...)
	})
}

// convertValueForSQL converts JSON-decoded values to proper SQL types.
// JSON unmarshaling can change types (e.g., int → float64), so we normalize them here.
func convertValueForSQL(val any) interface{} {
	switch v := val.(type) {
	case string:
		// Try to parse as time.Time if it's in RFC3339 format
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		return v

	case float64:
		// JSON numbers are always float64, but we might want int for integer columns
		// For now, pass as-is and let PostgreSQL handle the conversion
		return v

	case int:
		return v

	case int64:
		return v

	case bool:
		return v

	case time.Time:
		return v

	case nil:
		return nil

	default:
		// For unknown types, convert to string
		return fmt.Sprintf("%v", v)
	}
}
