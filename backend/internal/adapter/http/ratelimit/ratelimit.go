// Package ratelimit holds what the HTTP adapter needs to limit requests
// (ADR 0021): the rules and their numbers, the Redis keys, the pseudonyms
// of emails and addresses, and how to find the client's IP behind the
// proxies. The count
// itself is adapter/redis.RateLimiter.
package ratelimit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Limiter counts one attempt under key and reports whether it is allowed,
// and if not, how long until it would be (adapter/redis.RateLimiter). It
// never fails the caller: a count it cannot make is allowed and logged
// (ADR 0021).
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, wait time.Duration)
}

// Rule is one limit: at most Limit attempts per Kind of key in any Window.
type Rule struct {
	Name   string
	Kind   string
	Limit  int
	Window time.Duration
}

// The kinds of key a rule counts on.
const (
	KindIP         = "ip"
	KindEmail      = "email"
	KindUser       = "user"
	KindStore      = "store"
	KindStoreEmail = "store-email"
)

// The rules of ADR 0021. Per-IP rules run in middleware.RateLimits; the
// others run in the handlers, after decoding or after authn. They are read
// only: nothing assigns them, and a test pins every number to the ADR.
var (
	SignUpByIP                  = Rule{Name: "signup", Kind: KindIP, Limit: 10, Window: time.Hour}
	SignUpByEmail               = Rule{Name: "signup", Kind: KindEmail, Limit: 5, Window: time.Hour}
	LogInByIP                   = Rule{Name: "login", Kind: KindIP, Limit: 60, Window: 10 * time.Minute}
	LogInByEmail                = Rule{Name: "login", Kind: KindEmail, Limit: 10, Window: 10 * time.Minute}
	RefreshByIP                 = Rule{Name: "refresh", Kind: KindIP, Limit: 300, Window: 10 * time.Minute}
	PasswordResetByIP           = Rule{Name: "password-reset", Kind: KindIP, Limit: 10, Window: time.Hour}
	PasswordResetByEmail        = Rule{Name: "password-reset", Kind: KindEmail, Limit: 5, Window: time.Hour}
	PasswordResetConfirmByIP    = Rule{Name: "password-reset-confirm", Kind: KindIP, Limit: 30, Window: 10 * time.Minute}
	PasswordResetConfirmByEmail = Rule{Name: "password-reset-confirm", Kind: KindEmail, Limit: 10, Window: 10 * time.Minute}
	InvitationPreviewByIP       = Rule{Name: "invitation-preview", Kind: KindIP, Limit: 60, Window: 10 * time.Minute}
	InvitationAcceptByIP        = Rule{Name: "invitation-accept", Kind: KindIP, Limit: 20, Window: 10 * time.Minute}
	CreateTenantByUser          = Rule{Name: "create-tenant", Kind: KindUser, Limit: 5, Window: 24 * time.Hour}
	InviteByStore               = Rule{Name: "invite", Kind: KindStore, Limit: 20, Window: 24 * time.Hour}
	InviteByUser                = Rule{Name: "invite", Kind: KindUser, Limit: 50, Window: 24 * time.Hour}
	InviteByStoreEmail          = Rule{Name: "invite", Kind: KindStoreEmail, Limit: 3, Window: 24 * time.Hour}
)

// Rules returns every rule of ADR 0021, for checks that cover them all.
func Rules() []Rule {
	return []Rule{
		SignUpByIP, SignUpByEmail, LogInByIP, LogInByEmail, RefreshByIP,
		PasswordResetByIP, PasswordResetByEmail, PasswordResetConfirmByIP, PasswordResetConfirmByEmail,
		InvitationPreviewByIP, InvitationAcceptByIP, CreateTenantByUser,
		InviteByStore, InviteByUser, InviteByStoreEmail,
	}
}

// Key is the Redis key of rule for value: g:rl:{rule}:{kind}:{value}. Keys
// are global, because a limit is checked before or without a tenant.
func (r Rule) Key(value string) string {
	return "g:rl:" + r.Name + ":" + r.Kind + ":" + value
}

// MinKeyLength is the shortest key Pseudonyms accepts: shorter, and an HMAC
// of a known address list could be brute-forced.
const MinKeyLength = 32

// Pseudonyms turns emails and IP addresses into the values of keys, so
// neither sits in Redis in clear (ADR 0021). Each value is an HMAC-SHA256
// under a server key, in hex, with the kind of value mixed in so an email
// and an address never share one.
type Pseudonyms struct {
	key []byte
}

// NewPseudonyms returns the pseudonyms keyed with key, which step 2.6
// derives from the reset-code HMAC key. It returns an error for a key under
// MinKeyLength bytes: an empty key would make the HMAC a plain hash that a
// list of addresses reverses.
func NewPseudonyms(key []byte) (Pseudonyms, error) {
	if len(key) < MinKeyLength {
		return Pseudonyms{}, fmt.Errorf("ratelimit: the pseudonym key needs at least %d bytes, got %d", MinKeyLength, len(key))
	}
	return Pseudonyms{key: append([]byte(nil), key...)}, nil
}

// Email returns the value for email, from its lower-cased key, so changing
// the case of an email never buys more attempts (ADR 0010).
func (p Pseudonyms) Email(email identity.Email) string { return p.of("email", email.Key()) }

// StoreEmail returns the value of a store-and-email key.
func (p Pseudonyms) StoreEmail(store shared.TenantID, email identity.Email) string {
	return store.String() + ":" + p.Email(email)
}

// IP returns the value for a client address as ClientIP.Value gives it.
func (p Pseudonyms) IP(value string) string { return p.of("ip", value) }

func (p Pseudonyms) of(kind, value string) string {
	m := hmac.New(sha256.New, p.key)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}
