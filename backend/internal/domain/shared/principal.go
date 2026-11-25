package shared

import "fmt"

// Principal errors. Each matches ErrInvalid.
var (
	// ErrInvalidRole means a string is not one of the four staff roles.
	ErrInvalidRole = fmt.Errorf("invalid role: %w", ErrInvalid)
	// ErrInvalidPrincipal means the parts of a principal do not fit together,
	// such as a tenant without a role. The auth adapter treats it as a bad
	// token.
	ErrInvalidPrincipal = fmt.Errorf("invalid principal: %w", ErrInvalid)
)

// Audience says which kind of caller a token was issued to. It is the JWT
// "aud" claim (docs/architecture.md section 5.1).
type Audience string

// The two audiences. Client tokens are rejected on admin routes and staff
// tokens on the client area.
const (
	AudienceBackoffice Audience = "backoffice"
	AudienceClient     Audience = "client"
)

// Role is a staff member's role in one tenant. It lives here rather than in
// identity because Principal carries it and identity depends on shared; the
// permissions each role grants live in identity.
type Role string

// The staff roles (docs/architecture.md section 5.4).
const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleStaff  Role = "staff"
	RoleViewer Role = "viewer"
)

// ParseRole returns the role named s, or ErrInvalidRole.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if !r.Valid() {
		return "", ErrInvalidRole
	}
	return r, nil
}

// Valid reports whether r is one of the four staff roles.
func (r Role) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleStaff, RoleViewer:
		return true
	default:
		return false
	}
}

// Principal is the authenticated caller of a request, built by the auth
// adapter from a verified access token. A backoffice principal is a staff
// user, optionally acting in one tenant with one role; a client principal is
// a global client account with no tenant. The zero value is "nobody".
type Principal struct {
	audience        Audience
	userID          UserID
	tenantID        TenantID
	role            Role
	clientAccountID ClientAccountID
}

// NewBackofficePrincipal returns a staff principal. Tenant and role come
// together: both set when the session has picked a store, both empty when it
// has not. It returns ErrInvalidPrincipal for a zero user, a tenant without a
// valid role, or a role without a tenant.
func NewBackofficePrincipal(user UserID, tenant TenantID, role Role) (Principal, error) {
	if user.IsZero() {
		return Principal{}, fmt.Errorf("backoffice principal without a user: %w", ErrInvalidPrincipal)
	}
	if tenant.IsZero() != (role == "") {
		return Principal{}, fmt.Errorf("backoffice principal needs both tenant and role, or neither: %w", ErrInvalidPrincipal)
	}
	if role != "" && !role.Valid() {
		return Principal{}, fmt.Errorf("backoffice principal with role %q: %w", role, ErrInvalidPrincipal)
	}
	return Principal{audience: AudienceBackoffice, userID: user, tenantID: tenant, role: role}, nil
}

// NewClientPrincipal returns a client account principal. It returns
// ErrInvalidPrincipal for a zero account.
func NewClientPrincipal(account ClientAccountID) (Principal, error) {
	if account.IsZero() {
		return Principal{}, fmt.Errorf("client principal without an account: %w", ErrInvalidPrincipal)
	}
	return Principal{audience: AudienceClient, clientAccountID: account}, nil
}

// Audience returns who the token was issued to; empty for the zero value.
func (p Principal) Audience() Audience { return p.audience }

// UserID returns the staff user; zero for a client principal.
func (p Principal) UserID() UserID { return p.userID }

// TenantID returns the tenant the staff user acts in; zero when none is
// selected and for a client principal.
func (p Principal) TenantID() TenantID { return p.tenantID }

// Role returns the staff role in TenantID; empty without a tenant.
func (p Principal) Role() Role { return p.role }

// ClientAccountID returns the client account; zero for staff.
func (p Principal) ClientAccountID() ClientAccountID { return p.clientAccountID }

// HasTenant reports whether the principal acts inside a tenant.
func (p Principal) HasTenant() bool { return !p.tenantID.IsZero() }

// IsZero reports whether p is the zero value (no caller).
func (p Principal) IsZero() bool { return p == Principal{} }
