package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Users implements appidentity.Users on users. The table is global, with no
// RLS (0002), so it works in any scope.
type Users struct{}

// NewUsers returns the staff accounts repository.
func NewUsers() *Users { return &Users{} }

// InsertUser stores u. A second account for the same email, in any case, is
// an error matching shared.ErrConflict (the unique citext column).
func (*Users) InsertUser(ctx context.Context, u *identity.User) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.UserFromDomain(u)
	return wrap("insert user", db.Create(&m).Error)
}

// UserByID returns the account with this id.
func (*Users) UserByID(ctx context.Context, id shared.UserID) (*identity.User, error) {
	return takeUser(ctx, func(db *gorm.DB) *gorm.DB { return db.Where("id = ?", id.String()) })
}

// UsersByID returns the accounts that exist among ids.
func (*Users) UsersByID(ctx context.Context, ids []shared.UserID) ([]*identity.User, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = id.String()
	}
	var rows []models.User
	if err := db.Where("id IN ?", keys).Find(&rows).Error; err != nil {
		return nil, wrap("read users", err)
	}
	out := make([]*identity.User, 0, len(rows))
	for _, r := range rows {
		u, err := r.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// UserByEmail returns the account with this email; the column is citext, so
// the case does not matter.
func (*Users) UserByEmail(ctx context.Context, email identity.Email) (*identity.User, error) {
	return takeUser(ctx, func(db *gorm.DB) *gorm.DB { return db.Where("email = ?", email.String()) })
}

// LockUserByEmail is UserByEmail with the row locked FOR UPDATE.
func (*Users) LockUserByEmail(ctx context.Context, email identity.Email) (*identity.User, error) {
	return takeUser(ctx, func(db *gorm.DB) *gorm.DB { return db.Clauses(forUpdate).Where("email = ?", email.String()) })
}

func takeUser(ctx context.Context, query func(*gorm.DB) *gorm.DB) (*identity.User, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.User
	if err := query(db).Take(&m).Error; err != nil {
		return nil, wrap("read user", err)
	}
	return m.ToDomain()
}

// PasswordHashForShare reads the account's password hash FOR SHARE.
func (*Users) PasswordHashForShare(ctx context.Context, id shared.UserID) (identity.PasswordHash, error) {
	db, err := conn(ctx)
	if err != nil {
		return "", err
	}
	var m models.User
	if err := db.Clauses(forShare).Select("password_hash").Where("id = ?", id.String()).Take(&m).Error; err != nil {
		return "", wrap("read password hash", err)
	}
	return identity.PasswordHash(m.PasswordHash), nil
}

// UpdatePassword stores u's hash while the stored version is still
// u.Version(), and the next version.
func (*Users) UpdatePassword(ctx context.Context, u *identity.User) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Model(&models.User{}).Where("id = ? AND version = ?", u.ID().String(), u.Version()).
		Updates(map[string]any{
			"password_hash": string(u.PasswordHash()), "version": u.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	ok, err := affected(res, "update password")
	if err != nil {
		return err
	}
	if !ok {
		return shared.ErrVersionConflict
	}
	return nil
}

// RefreshTokens implements appidentity.RefreshTokens on refresh_families and
// refresh_tokens, following the lock protocol of ADR 0013. It handles staff
// families and tokens only: client ones come in milestone 7.
type RefreshTokens struct{}

// NewRefreshTokens returns the refresh tokens repository.
func NewRefreshTokens() *RefreshTokens { return &RefreshTokens{} }

const staffAudience = "audience = 'backoffice'"

// CreateFamily stores a new family.
func (*RefreshTokens) CreateFamily(ctx context.Context, f identity.RefreshFamily) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.FamilyFromDomain(f)
	return wrap("create refresh family", db.Create(&m).Error)
}

// InsertRefreshToken stores a new token of an existing family.
func (*RefreshTokens) InsertRefreshToken(ctx context.Context, t identity.RefreshToken) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.TokenFromDomain(t)
	return wrap("insert refresh token", db.Create(&m).Error)
}

// RefreshTokenByHash finds a staff token by its hash, with its family.
func (r *RefreshTokens) RefreshTokenByHash(ctx context.Context, hash shared.SecretHash) (identity.RefreshToken, identity.RefreshFamily, error) {
	db, err := conn(ctx)
	if err != nil {
		return identity.RefreshToken{}, identity.RefreshFamily{}, err
	}
	var m models.RefreshToken
	if err := db.Where("token_hash = ? AND "+staffAudience, hash[:]).Take(&m).Error; err != nil {
		return identity.RefreshToken{}, identity.RefreshFamily{}, wrap("read refresh token", err)
	}
	tok, err := m.ToDomain()
	if err != nil {
		return identity.RefreshToken{}, identity.RefreshFamily{}, err
	}
	fam, err := r.family(db, m.FamilyID)
	return tok, fam, err
}

// RefreshTokenByID reads a token again.
func (*RefreshTokens) RefreshTokenByID(ctx context.Context, id identity.RefreshTokenID) (identity.RefreshToken, error) {
	db, err := conn(ctx)
	if err != nil {
		return identity.RefreshToken{}, err
	}
	var m models.RefreshToken
	if err := db.Where("id = ? AND "+staffAudience, shared.ID(id).String()).Take(&m).Error; err != nil {
		return identity.RefreshToken{}, wrap("read refresh token", err)
	}
	return m.ToDomain()
}

// LockFamily locks the family row FOR UPDATE and returns it as it is then.
func (r *RefreshTokens) LockFamily(ctx context.Context, id identity.FamilyID) (identity.RefreshFamily, error) {
	db, err := conn(ctx)
	if err != nil {
		return identity.RefreshFamily{}, err
	}
	return r.family(db, shared.ID(id).String(), forUpdate)
}

func (*RefreshTokens) family(db *gorm.DB, id string, locks ...clause.Locking) (identity.RefreshFamily, error) {
	q := db.Where("id = ? AND "+staffAudience, id)
	for _, l := range locks {
		q = q.Clauses(l)
	}
	var m models.RefreshFamily
	if err := q.Take(&m).Error; err != nil {
		return identity.RefreshFamily{}, wrap("read refresh family", err)
	}
	return m.ToDomain()
}

// MarkRefreshTokenUsed sets used_at while the token is neither used nor
// superseded, and reports whether it did.
func (*RefreshTokens) MarkRefreshTokenUsed(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error) {
	return setOnce(ctx, id, "used_at", at, "mark refresh token used")
}

// SupersedeRefreshToken sets superseded_at under the same condition.
func (*RefreshTokens) SupersedeRefreshToken(ctx context.Context, id identity.RefreshTokenID, at time.Time) (bool, error) {
	return setOnce(ctx, id, "superseded_at", at, "supersede refresh token")
}

func setOnce(ctx context.Context, id identity.RefreshTokenID, column string, at time.Time, doing string) (bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return false, err
	}
	res := db.Model(&models.RefreshToken{}).
		Where("id = ? AND used_at IS NULL AND superseded_at IS NULL AND "+staffAudience, shared.ID(id).String()).
		Update(column, at)
	return affected(res, doing)
}

// SetSuccessor points a used token at next, a token of its family. It
// returns shared.ErrNotFound when no used token has this id.
func (*RefreshTokens) SetSuccessor(ctx context.Context, id, next identity.RefreshTokenID) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	res := db.Model(&models.RefreshToken{}).Where("id = ? AND used_at IS NOT NULL AND "+staffAudience, shared.ID(id).String()).
		Update("replaced_by", shared.ID(next).String())
	ok, err := affected(res, "set refresh token successor")
	if err != nil {
		return err
	}
	if !ok {
		return notFound("set refresh token successor")
	}
	return nil
}

// RevokeFamily revokes a live family at at, then reads, in a later
// statement, the jtis of its tokens created after issuedSince.
func (*RefreshTokens) RevokeFamily(ctx context.Context, id identity.FamilyID, at, issuedSince time.Time) ([]shared.ID, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	key := shared.ID(id).String()
	if err := db.Model(&models.RefreshFamily{}).Where("id = ? AND revoked_at IS NULL AND "+staffAudience, key).Update("revoked_at", at).Error; err != nil {
		return nil, wrap("revoke refresh family", err)
	}
	return jtis(db.Where("family_id = ? AND created_at > ? AND "+staffAudience, key, issuedSince))
}

// RevokeUserFamilies locks every family of the user, revokes the live ones,
// and reads the jtis in a later statement.
func (*RefreshTokens) RevokeUserFamilies(ctx context.Context, user shared.UserID, at, issuedSince time.Time) ([]shared.ID, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked []string
	if err := db.Model(&models.RefreshFamily{}).Clauses(forUpdate).Where("user_id = ? AND "+staffAudience, user.String()).
		Order("id").Pluck("id", &locked).Error; err != nil {
		return nil, wrap("lock refresh families", err)
	}
	if len(locked) == 0 {
		return []shared.ID{}, nil
	}
	if err := db.Model(&models.RefreshFamily{}).Where("id IN ? AND revoked_at IS NULL AND "+staffAudience, locked).Update("revoked_at", at).Error; err != nil {
		return nil, wrap("revoke refresh families", err)
	}
	return jtis(db.Where("family_id IN ? AND created_at > ? AND "+staffAudience, locked, issuedSince))
}

func jtis(q *gorm.DB) ([]shared.ID, error) {
	var raw []string
	if err := q.Model(&models.RefreshToken{}).Order("created_at, id").Pluck("access_jti", &raw).Error; err != nil {
		return nil, wrap("read access jtis", err)
	}
	out := make([]shared.ID, 0, len(raw))
	for _, s := range raw {
		id, err := shared.ParseID(s)
		if err != nil {
			return nil, wrap("read access jtis", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// ResetCodes implements appidentity.ResetCodes on password_reset_codes.
type ResetCodes struct{}

// NewResetCodes returns the password reset codes repository.
func NewResetCodes() *ResetCodes { return &ResetCodes{} }

// InsertResetCode stores a new code; the newest one is the user's code.
func (*ResetCodes) InsertResetCode(ctx context.Context, c identity.PasswordResetCode) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.ResetCodeFromDomain(c)
	return wrap("insert reset code", db.Create(&m).Error)
}

// NewestResetCode returns the user's most recent code.
func (*ResetCodes) NewestResetCode(ctx context.Context, user shared.UserID) (identity.PasswordResetCode, error) {
	db, err := conn(ctx)
	if err != nil {
		return identity.PasswordResetCode{}, err
	}
	var m models.ResetCode
	if err := db.Where("user_id = ?", user.String()).Order("created_at DESC, id DESC").Take(&m).Error; err != nil {
		return identity.PasswordResetCode{}, wrap("read reset code", err)
	}
	return m.ToDomain()
}

// WrongResetAttemptsSince sums the wrong tries of codes created after since.
func (*ResetCodes) WrongResetAttemptsSince(ctx context.Context, user shared.UserID, since time.Time) (int, error) {
	db, err := conn(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = db.Model(&models.ResetCode{}).Where("user_id = ? AND created_at > ?", user.String(), since).
		Select("COALESCE(SUM(attempts), 0)").Scan(&n).Error
	return n, wrap("sum wrong reset attempts", err)
}

// AddWrongResetAttempt counts one wrong try in one statement, only while the
// code is under identity.MaxResetAttempts.
func (*ResetCodes) AddWrongResetAttempt(ctx context.Context, id identity.ResetCodeID) (bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return false, err
	}
	res := db.Model(&models.ResetCode{}).Where("id = ? AND attempts < ?", shared.ID(id).String(), identity.MaxResetAttempts).
		Update("attempts", gorm.Expr("attempts + 1"))
	return affected(res, "add wrong reset attempt")
}

// MarkResetCodeUsed uses the code up while it is unused, unexpired at at and
// under identity.MaxResetAttempts.
func (*ResetCodes) MarkResetCodeUsed(ctx context.Context, id identity.ResetCodeID, at time.Time) (bool, error) {
	db, err := conn(ctx)
	if err != nil {
		return false, err
	}
	res := db.Model(&models.ResetCode{}).
		Where("id = ? AND used_at IS NULL AND expires_at > ? AND attempts < ?", shared.ID(id).String(), at, identity.MaxResetAttempts).
		Update("used_at", at)
	return affected(res, "mark reset code used")
}
