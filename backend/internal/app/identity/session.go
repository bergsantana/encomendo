package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// freshToken is a refresh token drawn before its transaction: the secret,
// the ids it is stored under, and the jti of the access token issued with
// it.
type freshToken struct {
	secret shared.Secret
	id     identity.RefreshTokenID
	family identity.FamilyID
	jti    shared.ID
}

func (s *Service) drawToken(ctx context.Context) (freshToken, error) {
	secret, err := s.secrets.NewSecret(ctx)
	if err != nil {
		return freshToken{}, fmt.Errorf("draw refresh token: %w", err)
	}
	return freshToken{
		secret: secret,
		id:     identity.RefreshTokenID(s.ids.NewID()),
		family: identity.FamilyID(s.ids.NewID()),
		jti:    s.ids.NewID(),
	}, nil
}

func (f freshToken) in(family identity.FamilyID, tenant shared.TenantID, now time.Time) (identity.RefreshToken, error) {
	return identity.NewRefreshToken(identity.RefreshTokenSpec{
		ID: f.id, Family: family, Tenant: tenant, Hash: f.secret.Hash(), AccessJTI: f.jti,
	}, now)
}

// startLogin stores a new family for user and its first token, for tenant.
func (s *Service) startLogin(ctx context.Context, user shared.UserID, tenant shared.TenantID, f freshToken, now time.Time) error {
	fam, err := identity.NewRefreshFamily(f.family, user)
	if err != nil {
		return err
	}
	if err := s.refresh.CreateFamily(ctx, fam); err != nil {
		return fmt.Errorf("create refresh family: %w", err)
	}
	tok, err := f.in(fam.ID, tenant, now)
	if err != nil {
		return err
	}
	if err := s.refresh.InsertRefreshToken(ctx, tok); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

// session is what a session-starting transaction read: the user, their
// stores and the one selected (zero for none).
type session struct {
	user        *identity.User
	memberships []Membership
	selected    Membership
}

func (sess session) view() SessionView {
	perms := []identity.Permission{}
	if !sess.selected.Tenant.ID.IsZero() {
		perms = identity.PermissionsOf(sess.selected.Role)
	}
	ms := sess.memberships
	if ms == nil {
		ms = []Membership{}
	}
	return SessionView{
		User: userView(sess.user), Tenant: sess.selected.Tenant, Role: sess.selected.Role,
		Permissions: perms, Memberships: ms,
	}
}

// issue signs the access token for sess, after its transaction committed.
// If signing fails the rotation stands and the client retries within the
// window (ADR 0013).
func (s *Service) issue(ctx context.Context, sess session, f freshToken, now time.Time) (AuthResult, error) {
	p, err := shared.NewBackofficePrincipal(sess.user.ID(), sess.selected.Tenant.ID, sess.selected.Role)
	if err != nil {
		// A membership without a valid role is a bug in tenancy, not a bad
		// token: keep it a 500 rather than ErrInvalidPrincipal's 401.
		return AuthResult{}, fmt.Errorf("session of %v: membership %+v does not make a principal: %s", sess.user.ID(), sess.selected, err.Error())
	}
	expires := now.Add(identity.AccessTokenLifetime)
	access, err := s.tokens.Issue(ctx, AccessClaims{Principal: p, JTI: f.jti, IssuedAt: now, ExpiresAt: expires})
	if err != nil {
		return AuthResult{}, fmt.Errorf("issue access token: %w", err)
	}
	return AuthResult{
		Session: sess.view(), AccessToken: access, AccessExpiresAt: expires,
		RefreshToken: f.secret.Reveal(), RefreshExpiresAt: now.Add(identity.RefreshTokenLifetime),
	}, nil
}

// SignUp creates a staff account and logs it in, with no store selected
// (ADR 0010). It returns a ValidationError listing every invalid field and
// identity.ErrEmailTaken when the email has an account.
func (s *Service) SignUp(ctx context.Context, in SignUpInput) (AuthResult, error) {
	name, nameErr := identity.ParsePersonName(in.Name)
	email, emailErr := identity.ParseEmail(in.Email)
	if err := shared.MergeValidation(nameErr, emailErr, identity.CheckNewPassword(in.Password)); err != nil {
		return AuthResult{}, err
	}
	hash, err := s.hasher.Hash(ctx, in.Password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := identity.NewUser(shared.UserID(s.ids.NewID()), identity.Profile{Name: name, Email: email}, hash)
	if err != nil {
		return AuthResult{}, err
	}
	fresh, err := s.drawToken(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	now := s.clock.Now()
	err = s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
		if err := s.insertUser(ctx, user); err != nil {
			return err
		}
		return s.startLogin(ctx, user.ID(), shared.TenantID{}, fresh, now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, session{user: user}, fresh, now)
}

func (s *Service) insertUser(ctx context.Context, u *identity.User) error {
	err := s.users.InsertUser(ctx, u)
	if errors.Is(err, shared.ErrConflict) {
		return identity.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// Login checks email and password and starts a session. It selects
// in.TenantID when the user belongs to it, otherwise the user's only store,
// otherwise none (docs/api/openapi.yaml, logIn). It returns
// identity.ErrInvalidCredentials for an unknown email and a wrong password
// alike, and a ValidationError for a malformed email or an overlong
// password.
func (s *Service) Login(ctx context.Context, in LoginInput) (AuthResult, error) {
	email, err := identity.ParseEmail(in.Email)
	if err := shared.MergeValidation(err, identity.CheckPresentedPassword(in.Password)); err != nil {
		return AuthResult{}, err
	}
	user, err := s.findUser(ctx, email)
	if err != nil {
		return AuthResult{}, err
	}
	verified, err := s.checkPassword(ctx, user, in.Password)
	if err != nil {
		return AuthResult{}, err
	}
	fresh, err := s.drawToken(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	now := s.clock.Now()
	var sess session
	err = s.tx.WithinTx(userScope(ctx, user.ID()), func(ctx context.Context) error {
		if err := s.recheckPassword(ctx, user.ID(), verified); err != nil {
			return err
		}
		ms, err := s.memberships.MembershipsOf(ctx, user.ID())
		if err != nil {
			return fmt.Errorf("read memberships: %w", err)
		}
		sess = session{user: user, memberships: ms, selected: chooseAtLogin(ms, in.TenantID)}
		return s.startLogin(ctx, user.ID(), sess.selected.Tenant.ID, fresh, now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, sess, fresh, now)
}

// chooseAtLogin selects the hinted store when the user is in it, else the
// only store, else none. A suspended store is never selected (ADR 0022):
// not by a hint, and not as the only store. A user with one active and one
// suspended store still has two stores, so gets the picker.
func chooseAtLogin(ms []Membership, hint shared.TenantID) Membership {
	if m, ok := find(ms, hint); ok {
		return m
	}
	if len(ms) == 1 && !ms[0].Tenant.Suspended {
		return ms[0]
	}
	return Membership{}
}

// findUser returns the account with email, or nil when there is none.
func (s *Service) findUser(ctx context.Context, email identity.Email) (*identity.User, error) {
	var user *identity.User
	err := s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
		u, err := s.users.UserByEmail(ctx, email)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find user: %w", err)
		}
		user = u
		return nil
	})
	return user, err
}

// checkPassword runs argon2id outside any transaction and returns the hash
// it verified. For no user it checks against no hash, which costs the same
// (ADR 0010), and then refuses.
func (s *Service) checkPassword(ctx context.Context, u *identity.User, password string) (identity.PasswordHash, error) {
	var hash identity.PasswordHash
	if u != nil {
		hash = u.PasswordHash()
	}
	ok, err := s.hasher.Verify(ctx, hash, password)
	if err != nil {
		return "", fmt.Errorf("verify password: %w", err)
	}
	if !ok || u == nil {
		return "", identity.ErrInvalidCredentials
	}
	return hash, nil
}

// recheckPassword reads the hash again FOR SHARE in the transaction that
// creates the session and refuses when a reset changed it since it was
// verified. A reset that comes first then makes this login fail; one that
// comes after waits and revokes the new family (ADR 0013).
func (s *Service) recheckPassword(ctx context.Context, user shared.UserID, verified identity.PasswordHash) error {
	current, err := s.users.PasswordHashForShare(ctx, user)
	if errors.Is(err, shared.ErrNotFound) {
		return identity.ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("read password hash again: %w", err)
	}
	if current != verified {
		return identity.ErrInvalidCredentials
	}
	return nil
}
