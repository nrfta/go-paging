package paging

import "fmt"

// Connection represents a Relay-compliant GraphQL connection.
// It provides both edges (with cursors) and nodes (direct access) to support
// different query patterns. This follows the Relay specification while being
// flexible enough for various use cases.
//
// Type parameter T is the domain model type (e.g., User, Post, Organization).
//
// Example GraphQL schema:
//
//	type UserConnection {
//	  edges: [UserEdge!]!
//	  nodes: [User!]!
//	  pageInfo: PageInfo!
//	}
type Connection[T any] struct {
	// Edges contains the list of edges, each with a cursor and node.
	// Use this when clients need individual item cursors.
	Edges []Edge[T] `json:"edges"`

	// Nodes provides direct access to the items without cursor overhead.
	// Use this for simpler queries that only need the data.
	Nodes []T `json:"nodes"`

	// PageInfo contains pagination metadata (hasNextPage, cursors, etc.)
	PageInfo PageInfo `json:"pageInfo"`
}

// Edge represents a Relay-compliant edge in a connection.
// Each edge contains a cursor (for pagination) and the node (actual data).
//
// Type parameter T is the domain model type.
//
// Example GraphQL schema:
//
//	type UserEdge {
//	  cursor: String!
//	  node: User!
//	}
type Edge[T any] struct {
	// Cursor is an opaque string that marks this item's position in the list.
	// Clients can use this cursor to resume pagination from this point.
	Cursor string `json:"cursor"`

	// Node is the actual data item.
	Node T `json:"node"`
}

// BuildConnection creates a Connection from a slice of source items.
// It handles transformation from database models to domain models and
// automatically generates cursors for each item.
//
// Type parameters:
//   - From: Source type (e.g., SQLBoiler model, database row)
//   - To: Target type (e.g., domain model, GraphQL type)
//
// Parameters:
//   - items: Slice of source items to transform
//   - pageInfo: Pagination metadata (hasNextPage, totalCount, etc.)
//   - cursorEncoder: Function that generates a cursor for each item
//   - transform: Function that converts From -> To (can return error)
//
// Returns the built Connection or an error if transformation fails.
//
// Example usage:
//
//	conn, err := paging.BuildConnection(
//	    dbRecords,
//	    pageInfo,
//	    func(i int, item *models.User) string {
//	        return offset.EncodeCursor(startOffset + i + 1)
//	    },
//	    func(item *models.User) (*domain.User, error) {
//	        return toDomainUser(item)
//	    },
//	)
func BuildConnection[From any, To any](
	items []From,
	pageInfo PageInfo,
	cursorEncoder func(index int, item From) string,
	transform func(From) (To, error),
) (*Connection[To], error) {
	conn := &Connection[To]{
		Nodes:    make([]To, 0, len(items)),
		Edges:    make([]Edge[To], 0, len(items)),
		PageInfo: pageInfo,
	}

	for i, item := range items {
		transformed, err := transform(item)
		if err != nil {
			return nil, fmt.Errorf("transform item at index %d: %w", i, err)
		}

		cursor := cursorEncoder(i, item)
		conn.Nodes = append(conn.Nodes, transformed)
		conn.Edges = append(conn.Edges, Edge[To]{
			Cursor: cursor,
			Node:   transformed,
		})
	}

	return conn, nil
}
