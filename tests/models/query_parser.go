package models

import (
	"fmt"
	"strings"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
)

// QueryParams holds parsed query parameters extracted from SQLBoiler query mods.
type QueryParams struct {
	Where   string
	OrderBy string
	Offset  int
	Limit   int
}

// ParseQueryMods extracts query parameters from SQLBoiler query mods.
// It parses the string representation of mods to identify WHERE, ORDER BY,
// OFFSET, and LIMIT clauses.
//
// Numeric mod ordering follows OffsetToQueryMods: Offset (if > 0) comes before Limit.
func ParseQueryMods(mods []qm.QueryMod) QueryParams {
	var params QueryParams
	var numbers []int

	for _, mod := range mods {
		str := strings.Trim(fmt.Sprintf("%v", mod), "{}")

		switch {
		case strings.HasPrefix(str, "WHERE "):
			params.Where = strings.TrimPrefix(str, "WHERE ")

		case isWhereClause(str):
			params.Where = strings.TrimSuffix(str, " []")

		case isOrderByClause(str):
			params.OrderBy = strings.TrimSuffix(str, " []")

		default:
			var val int
			if _, err := fmt.Sscanf(str, "%d", &val); err == nil {
				numbers = append(numbers, val)
			}
		}
	}

	// Assign numeric values: first is Offset (if two present), second is Limit
	switch len(numbers) {
	case 1:
		params.Limit = numbers[0]
	case 2:
		params.Offset = numbers[0]
		params.Limit = numbers[1]
	}

	return params
}

func isOrderByClause(s string) bool {
	return strings.Contains(s, " DESC") || strings.Contains(s, " ASC")
}

func isWhereClause(s string) bool {
	return strings.Contains(s, " IS ") ||
		strings.Contains(s, "=") ||
		strings.Contains(s, ">") ||
		strings.Contains(s, "<")
}

// BuildSelectQuery constructs a SELECT query with the given table, columns, and params.
func BuildSelectQuery(table, columns string, params QueryParams) string {
	query := fmt.Sprintf("SELECT %s FROM %s", columns, table)
	if params.Where != "" {
		query += " WHERE " + params.Where
	}
	if params.OrderBy != "" {
		query += " ORDER BY " + params.OrderBy
	}
	if params.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", params.Limit)
	}
	if params.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", params.Offset)
	}
	return query
}

// BuildCountQuery constructs a COUNT query with the given table and params.
func BuildCountQuery(table string, params QueryParams) string {
	query := fmt.Sprintf("SELECT count(*) FROM %s", table)
	if params.Where != "" {
		query += " WHERE " + params.Where
	}
	return query
}
