package identity

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrInvalidUser means a user was built from missing or malformed parts.
// Input reaches NewUser already parsed and rows reach RestoreUser from the
// database, so this is a bug, not bad input: it matches no sentinel and the
// API answers 500.
var ErrInvalidUser = errors.New("identity: invalid user")

// Profile is who a staff user is to others: name and email.
type Profile struct {
	Name  PersonName
	Email Email
}

// User is a staff account: an owner, admin, staff member or viewer of any
// number of stores. Which stores, and with which role, is tenancy's
// business. Accounts are never deleted in milestone 2.
type User struct {
	id           shared.UserID
	profile      Profile
	passwordHash PasswordHash
	version      int
}

// NewUser returns a new account at version 1. It returns ErrInvalidUser for
// a zero id, name or email, or an empty hash.
func NewUser(id shared.UserID, p Profile, hash PasswordHash) (*User, error) {
	if id.IsZero() || p.Name.IsZero() || p.Email.IsZero() || hash == "" {
		return nil, fmt.Errorf("new user %v: %w", id, ErrInvalidUser)
	}
	return &User{id: id, profile: p, passwordHash: hash, version: 1}, nil
}

// UserRecord is a user as a repository stores it.
type UserRecord struct {
	ID           shared.UserID
	Name         string
	Email        string
	PasswordHash PasswordHash
	Version      int
}

// RestoreUser rebuilds a stored user. It returns ErrInvalidUser when the
// record breaks a rule a stored user always keeps.
func RestoreUser(r UserRecord) (*User, error) {
	name, nameErr := ParsePersonName(r.Name)
	email, emailErr := ParseEmail(r.Email)
	if nameErr != nil || emailErr != nil || r.Version < 1 {
		return nil, fmt.Errorf("restore user %v: %w", r.ID, ErrInvalidUser)
	}
	u, err := NewUser(r.ID, Profile{Name: name, Email: email}, r.PasswordHash)
	if err != nil {
		return nil, err
	}
	u.version = r.Version
	return u, nil
}

// ID returns the user's id.
func (u *User) ID() shared.UserID { return u.id }

// Name returns the user's name.
func (u *User) Name() PersonName { return u.profile.Name }

// Email returns the address the user logs in with.
func (u *User) Email() Email { return u.profile.Email }

// Profile returns name and email together.
func (u *User) Profile() Profile { return u.profile }

// PasswordHash returns the stored hash of the user's password.
func (u *User) PasswordHash() PasswordHash { return u.passwordHash }

// Version returns the version the user was read or created at. A
// repository's update stores the next one, and only while the stored version
// is still this one.
func (u *User) Version() int { return u.version }

// ChangePassword replaces the password hash (a password reset). It returns
// ErrInvalidUser for an empty hash and then changes nothing. The caller also
// revokes every session of the account (ADR 0011).
func (u *User) ChangePassword(hash PasswordHash) error {
	if hash == "" {
		return fmt.Errorf("change password of %v: %w", u.id, ErrInvalidUser)
	}
	u.passwordHash = hash
	return nil
}
