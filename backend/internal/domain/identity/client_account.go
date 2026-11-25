package identity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrInvalidClientAccount means a client account or identity was built from
// missing or malformed parts. It is a bug, so it matches no sentinel.
var ErrInvalidClientAccount = errors.New("identity: invalid client account")

// ClientAccount is a client's global account, used on every store page and
// in Minhas encomendas (docs/architecture.md 5.3). It is optional, has no
// password and no role, and belongs to no store. Its login methods are
// ClientIdentity values; the login flows come with milestone 7.
type ClientAccount struct {
	id      shared.ClientAccountID
	name    string
	version int
}

// NewClientAccount returns an account at version 1. The name is optional:
// blank means none. It returns a ValidationError on "name" for a name
// ParsePersonName refuses, and ErrInvalidClientAccount for a zero id.
func NewClientAccount(id shared.ClientAccountID, name string) (*ClientAccount, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("new client account: %w", ErrInvalidClientAccount)
	}
	a := &ClientAccount{id: id, version: 1}
	if err := a.Rename(name); err != nil {
		return nil, err
	}
	return a, nil
}

// ID returns the account id.
func (a *ClientAccount) ID() shared.ClientAccountID { return a.id }

// Name returns the client's name, or "" when they gave none.
func (a *ClientAccount) Name() string { return a.name }

// Version returns the version the account was read or created at.
func (a *ClientAccount) Version() int { return a.version }

// Rename sets the name; blank clears it. It returns a ValidationError on
// "name" and changes nothing when ParsePersonName refuses the name.
func (a *ClientAccount) Rename(name string) error {
	if strings.TrimSpace(name) == "" {
		a.name = ""
		return nil
	}
	n, err := ParsePersonName(name)
	if err != nil {
		return err
	}
	a.name = n.String()
	return nil
}

// ClientIdentityKind is how a client logs in.
type ClientIdentityKind string

// The two ways (0002_identity: client_identities).
const (
	ByEmail  ClientIdentityKind = "email"
	ByGoogle ClientIdentityKind = "google"
)

const maxGoogleSubLength = 255

// ClientIdentity is one way to log in to a client account: a verified email
// (one-time codes) or a Google account's subject. Each identity belongs to
// one account, and an email or sub to at most one identity.
type ClientIdentity struct {
	account   shared.ClientAccountID
	kind      ClientIdentityKind
	email     Email
	googleSub string
}

// NewEmailIdentity returns an email identity. The email must already be
// verified by a login code. It returns ErrInvalidClientAccount for a zero
// account or email.
func NewEmailIdentity(account shared.ClientAccountID, email Email) (ClientIdentity, error) {
	if account.IsZero() || email.IsZero() {
		return ClientIdentity{}, fmt.Errorf("new email identity: %w", ErrInvalidClientAccount)
	}
	return ClientIdentity{account: account, kind: ByEmail, email: email}, nil
}

// NewGoogleIdentity returns a Google identity for the "sub" of a verified ID
// token: 1 to 255 visible ASCII characters. It returns
// ErrInvalidClientAccount for a zero account or any other sub.
func NewGoogleIdentity(account shared.ClientAccountID, sub string) (ClientIdentity, error) {
	if account.IsZero() || !visibleASCII(sub, maxGoogleSubLength) {
		return ClientIdentity{}, fmt.Errorf("new google identity: %w", ErrInvalidClientAccount)
	}
	return ClientIdentity{account: account, kind: ByGoogle, googleSub: sub}, nil
}

func visibleASCII(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for i := range len(s) {
		if s[i] < '!' || s[i] > '~' {
			return false
		}
	}
	return true
}

// Account returns the account the identity logs in to.
func (i ClientIdentity) Account() shared.ClientAccountID { return i.account }

// Kind returns how the identity logs in.
func (i ClientIdentity) Kind() ClientIdentityKind { return i.kind }

// Email returns the verified email; zero for a Google identity.
func (i ClientIdentity) Email() Email { return i.email }

// GoogleSub returns the Google subject; empty for an email identity.
func (i ClientIdentity) GoogleSub() string { return i.googleSub }
