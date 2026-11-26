// Package models holds the GORM persistence models of the Postgres adapter.
// Domain structs never carry ORM tags (backend/CLAUDE.md, GORM): each model
// here maps one table, with FromDomain and ToDomain converting to and from
// the aggregate. Updates name their columns, because encomendo_app has
// UPDATE only on some of them (0002, 0007).
package models

import (
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// User maps users.
type User struct {
	ID           string `gorm:"column:id;primaryKey"`
	Email        string `gorm:"column:email"`
	Name         string `gorm:"column:name"`
	PasswordHash string `gorm:"column:password_hash"`
	Version      int    `gorm:"column:version"`
}

// TableName returns users.
func (User) TableName() string { return "users" }

// UserFromDomain returns the row of u.
func UserFromDomain(u *identity.User) User {
	return User{
		ID: u.ID().String(), Email: u.Email().String(), Name: u.Name().String(),
		PasswordHash: string(u.PasswordHash()), Version: u.Version(),
	}
}

// ToDomain rebuilds the account.
func (m User) ToDomain() (*identity.User, error) {
	id, err := parseID[shared.UserID](m.ID)
	if err != nil {
		return nil, err
	}
	return identity.RestoreUser(identity.UserRecord{
		ID: id, Name: m.Name, Email: m.Email, PasswordHash: identity.PasswordHash(m.PasswordHash), Version: m.Version,
	})
}

// RefreshFamily maps refresh_families, staff families only.
type RefreshFamily struct {
	ID        string     `gorm:"column:id;primaryKey"`
	Audience  string     `gorm:"column:audience"`
	UserID    string     `gorm:"column:user_id"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

// TableName returns refresh_families.
func (RefreshFamily) TableName() string { return "refresh_families" }

// FamilyFromDomain returns the row of a staff family.
func FamilyFromDomain(f identity.RefreshFamily) RefreshFamily {
	return RefreshFamily{
		ID: shared.ID(f.ID).String(), Audience: string(shared.AudienceBackoffice), UserID: f.User.String(),
		RevokedAt: nullTime(f.RevokedAt),
	}
}

// ToDomain rebuilds the family.
func (m RefreshFamily) ToDomain() (identity.RefreshFamily, error) {
	id, err := parseID[identity.FamilyID](m.ID)
	if err != nil {
		return identity.RefreshFamily{}, err
	}
	user, err := parseID[shared.UserID](m.UserID)
	if err != nil {
		return identity.RefreshFamily{}, err
	}
	return identity.RefreshFamily{ID: id, User: user, RevokedAt: zeroTime(m.RevokedAt)}, nil
}

// RefreshToken maps refresh_tokens, staff tokens only.
type RefreshToken struct {
	ID           string     `gorm:"column:id;primaryKey"`
	FamilyID     string     `gorm:"column:family_id"`
	Audience     string     `gorm:"column:audience"`
	TenantID     *string    `gorm:"column:tenant_id"`
	TokenHash    []byte     `gorm:"column:token_hash"`
	AccessJTI    string     `gorm:"column:access_jti"`
	ExpiresAt    time.Time  `gorm:"column:expires_at"`
	UsedAt       *time.Time `gorm:"column:used_at"`
	ReplacedBy   *string    `gorm:"column:replaced_by"`
	SupersededAt *time.Time `gorm:"column:superseded_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

// TableName returns refresh_tokens.
func (RefreshToken) TableName() string { return "refresh_tokens" }

// TokenFromDomain returns the row of a staff token.
func TokenFromDomain(t identity.RefreshToken) RefreshToken {
	m := RefreshToken{
		ID: shared.ID(t.ID).String(), FamilyID: shared.ID(t.Family).String(), Audience: string(shared.AudienceBackoffice),
		TokenHash: t.Hash[:], AccessJTI: t.AccessJTI.String(), ExpiresAt: t.ExpiresAt, UsedAt: nullTime(t.UsedAt),
		SupersededAt: nullTime(t.SupersededAt), CreatedAt: t.CreatedAt,
	}
	if !t.Tenant.IsZero() {
		s := t.Tenant.String()
		m.TenantID = &s
	}
	if !shared.ID(t.ReplacedBy).IsZero() {
		s := shared.ID(t.ReplacedBy).String()
		m.ReplacedBy = &s
	}
	return m
}

// ToDomain rebuilds the token.
func (m RefreshToken) ToDomain() (identity.RefreshToken, error) {
	id, err := parseID[identity.RefreshTokenID](m.ID)
	if err != nil {
		return identity.RefreshToken{}, err
	}
	family, err := parseID[identity.FamilyID](m.FamilyID)
	if err != nil {
		return identity.RefreshToken{}, err
	}
	jti, err := shared.ParseID(m.AccessJTI)
	if err != nil {
		return identity.RefreshToken{}, err
	}
	if len(m.TokenHash) != len(shared.SecretHash{}) {
		return identity.RefreshToken{}, fmt.Errorf("refresh token %s: stored hash is %d bytes", m.ID, len(m.TokenHash))
	}
	t := identity.RefreshToken{
		ID: id, Family: family, AccessJTI: jti, ExpiresAt: m.ExpiresAt.UTC(), CreatedAt: m.CreatedAt.UTC(),
		UsedAt: zeroTime(m.UsedAt), SupersededAt: zeroTime(m.SupersededAt),
	}
	copy(t.Hash[:], m.TokenHash)
	if m.TenantID != nil {
		if t.Tenant, err = parseID[shared.TenantID](*m.TenantID); err != nil {
			return identity.RefreshToken{}, err
		}
	}
	if m.ReplacedBy != nil {
		if t.ReplacedBy, err = parseID[identity.RefreshTokenID](*m.ReplacedBy); err != nil {
			return identity.RefreshToken{}, err
		}
	}
	return t, nil
}

// ResetCode maps password_reset_codes.
type ResetCode struct {
	ID        string     `gorm:"column:id;primaryKey"`
	UserID    string     `gorm:"column:user_id"`
	CodeHash  []byte     `gorm:"column:code_hash"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	Attempts  int        `gorm:"column:attempts"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

// TableName returns password_reset_codes.
func (ResetCode) TableName() string { return "password_reset_codes" }

// ResetCodeFromDomain returns the row of c.
func ResetCodeFromDomain(c identity.PasswordResetCode) ResetCode {
	return ResetCode{
		ID: shared.ID(c.ID).String(), UserID: c.User.String(), CodeHash: c.Hash[:], ExpiresAt: c.ExpiresAt,
		Attempts: c.Attempts, UsedAt: nullTime(c.UsedAt), CreatedAt: c.CreatedAt,
	}
}

// ToDomain rebuilds the code.
func (m ResetCode) ToDomain() (identity.PasswordResetCode, error) {
	id, err := parseID[identity.ResetCodeID](m.ID)
	if err != nil {
		return identity.PasswordResetCode{}, err
	}
	user, err := parseID[shared.UserID](m.UserID)
	if err != nil {
		return identity.PasswordResetCode{}, err
	}
	if len(m.CodeHash) != len(identity.CodeHash{}) {
		return identity.PasswordResetCode{}, fmt.Errorf("reset code %s: stored hash is %d bytes", m.ID, len(m.CodeHash))
	}
	c := identity.PasswordResetCode{
		ID: id, User: user, CreatedAt: m.CreatedAt.UTC(), ExpiresAt: m.ExpiresAt.UTC(), Attempts: m.Attempts,
		UsedAt: zeroTime(m.UsedAt),
	}
	copy(c.Hash[:], m.CodeHash)
	return c, nil
}

// parseID reads a stored uuid as the typed id T.
func parseID[T ~[16]byte](s string) (T, error) {
	id, err := shared.ParseID(s)
	if err != nil {
		return T{}, fmt.Errorf("stored id: %w", err)
	}
	return T(id), nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func zeroTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}
