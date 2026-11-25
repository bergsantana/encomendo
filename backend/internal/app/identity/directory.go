package identity

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Directory answers other contexts' questions about staff users: tenancy
// asks who an email belongs to when inviting, and who its members are when
// listing them. adapter/inproc implements tenancy's port on top of it (ADR
// 0011). It depends only on identity's own repository, so main.go builds it
// before either service and no context depends on the other's Service. Its
// methods run inside the caller's transaction.
type Directory struct {
	users Users
}

// NewDirectory returns a Directory over users.
func NewDirectory(users Users) *Directory { return &Directory{users: users} }

// UserByEmail returns the account with this email, ignoring case, or an
// error matching shared.ErrNotFound.
func (d *Directory) UserByEmail(ctx context.Context, email identity.Email) (UserView, error) {
	u, err := d.users.UserByEmail(ctx, email)
	if err != nil {
		return UserView{}, fmt.Errorf("find user by email: %w", err)
	}
	return userView(u), nil
}

// UsersByID returns the accounts with these ids that exist, in any order.
func (d *Directory) UsersByID(ctx context.Context, ids []shared.UserID) ([]UserView, error) {
	users, err := d.users.UsersByID(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("find users: %w", err)
	}
	out := make([]UserView, 0, len(users))
	for _, u := range users {
		out = append(out, userView(u))
	}
	return out, nil
}
