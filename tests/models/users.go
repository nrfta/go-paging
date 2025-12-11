package models

import (
	"context"
	"time"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/aarondl/sqlboiler/v4/queries/qm"
)

// User is an object representing the database table.
type User struct {
	ID        string    `boil:"id" json:"id"`
	Email     string    `boil:"email" json:"email"`
	Name      string    `boil:"name" json:"name"`
	Age       int       `boil:"age" json:"age"`
	IsActive  bool      `boil:"is_active" json:"is_active"`
	CreatedAt time.Time `boil:"created_at" json:"created_at"`
	UpdatedAt time.Time `boil:"updated_at" json:"updated_at"`
}

const (
	userTable   = "users"
	userColumns = "id, email, name, age, is_active, created_at, updated_at"
)

type userQuery struct {
	mods []qm.QueryMod
}

// Users returns a new query against the users table.
func Users(mods ...qm.QueryMod) userQuery {
	return userQuery{mods: mods}
}

// All returns all User records from the query.
func (q userQuery) All(ctx context.Context, exec boil.ContextExecutor) ([]*User, error) {
	params := ParseQueryMods(q.mods)
	query := BuildSelectQuery(userTable, userColumns, params)

	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Age, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// Count returns the count of all User records in the query.
func (q userQuery) Count(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	params := ParseQueryMods(q.mods)
	query := BuildCountQuery(userTable, params)

	var count int64
	err := exec.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}
