package sqlboiler

import (
	"strings"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/nrfta/go-paging"
)

// OffsetToQueryMods converts FetchParams into SQLBoiler query mods for offset pagination.
// This is the strategy-specific query builder for offset-based pagination.
//
// The conversion follows these rules:
//   - Offset → qm.Offset(n)
//   - Limit → qm.Limit(n)
//   - OrderBy → qm.OrderBy("col1 DESC, col2 ASC")
//
// This function is used by offset.Paginator when creating a SQLBoiler fetcher.
//
// Example:
//
//	fetcher := sqlboiler.NewFetcher(
//	    queryFunc,
//	    countFunc,
//	    sqlboiler.OffsetToQueryMods, // ← Use offset strategy
//	)
func OffsetToQueryMods(params paging.FetchParams) []qm.QueryMod {
	mods := []qm.QueryMod{}

	if params.Offset > 0 {
		mods = append(mods, qm.Offset(params.Offset))
	}

	if params.Limit > 0 {
		mods = append(mods, qm.Limit(params.Limit))
	}

	if len(params.OrderBy) > 0 {
		mods = append(mods, qm.OrderBy(buildOrderByClause(params.OrderBy)))
	}

	return mods
}

// buildOrderByClause constructs an ORDER BY clause from OrderBy directives.
// Assumes len(orderBy) > 0 (caller must verify).
//
// Example:
//
//	[]OrderBy{
//	    {Column: "created_at", Desc: true},
//	    {Column: "id", Desc: false},
//	}
//	→ "created_at DESC, id"
func buildOrderByClause(orderBy []paging.OrderBy) string {
	parts := make([]string, len(orderBy))
	for i, o := range orderBy {
		if o.Desc {
			parts[i] = o.Column + " DESC"
		} else {
			parts[i] = o.Column
		}
	}
	return strings.Join(parts, ", ")
}
