package identity

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func mustEmail(t *testing.T, s string) Email {
	t.Helper()
	e, err := ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustName(t *testing.T, s string) PersonName {
	t.Helper()
	n, err := ParsePersonName(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

var userID = shared.UserID(shared.NewV7(1_700_000_000_000, [10]byte{1}))

func TestNewUser(t *testing.T) {
	t.Parallel()

	u, err := NewUser(userID, Profile{Name: mustName(t, "Ana Souza"), Email: mustEmail(t, "ana@altarpm.com.br")}, "$argon2id$hash")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID() != userID || u.Name().String() != "Ana Souza" || u.Email().String() != "ana@altarpm.com.br" {
		t.Errorf("user = %v %q %q", u.ID(), u.Name(), u.Email())
	}
	if u.PasswordHash() != "$argon2id$hash" || u.Version() != 1 {
		t.Errorf("hash %q, version %d; want the hash and version 1", u.PasswordHash(), u.Version())
	}
}

func TestNewUser_RefusesMissingParts(t *testing.T) {
	t.Parallel()

	profile := Profile{Name: mustName(t, "Ana"), Email: mustEmail(t, "ana@ex.com")}
	cases := map[string]func() (*User, error){
		"zero id":    func() (*User, error) { return NewUser(shared.UserID{}, profile, "h") },
		"zero name":  func() (*User, error) { return NewUser(userID, Profile{Email: profile.Email}, "h") },
		"zero email": func() (*User, error) { return NewUser(userID, Profile{Name: profile.Name}, "h") },
		"no hash":    func() (*User, error) { return NewUser(userID, profile, "") },
	}
	for name, build := range cases {
		if _, err := build(); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%s: err = %v, want ErrInvalidUser", name, err)
		}
	}
}

func TestErrInvalidUser_IsABugNotBadInput(t *testing.T) {
	t.Parallel()

	// Parsed values reach NewUser, so a refusal means the calling code is
	// wrong: a 500, never a 400.
	if errors.Is(ErrInvalidUser, shared.ErrInvalid) {
		t.Error("ErrInvalidUser matches ErrInvalid")
	}
}

func TestRestoreUser(t *testing.T) {
	t.Parallel()

	u, err := RestoreUser(UserRecord{ID: userID, Name: "Ana", Email: "ana@ex.com", PasswordHash: "h", Version: 4})
	if err != nil {
		t.Fatal(err)
	}
	if u.Version() != 4 || u.Email().String() != "ana@ex.com" {
		t.Errorf("restored version %d, email %q", u.Version(), u.Email())
	}
	for name, rec := range map[string]UserRecord{
		"bad email":    {ID: userID, Name: "Ana", Email: "nope", PasswordHash: "h", Version: 1},
		"bad name":     {ID: userID, Name: "", Email: "ana@ex.com", PasswordHash: "h", Version: 1},
		"zero version": {ID: userID, Name: "Ana", Email: "ana@ex.com", PasswordHash: "h"},
		"no hash":      {ID: userID, Name: "Ana", Email: "ana@ex.com", Version: 1},
	} {
		if _, err := RestoreUser(rec); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%s: err = %v, want ErrInvalidUser", name, err)
		}
	}
}

func TestUser_ChangePassword(t *testing.T) {
	t.Parallel()

	u, _ := RestoreUser(UserRecord{ID: userID, Name: "Ana", Email: "ana@ex.com", PasswordHash: "old", Version: 2})
	if err := u.ChangePassword("new"); err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash() != "new" {
		t.Errorf("hash = %q after ChangePassword", u.PasswordHash())
	}
	// The version is the one read; the repository stores the next one.
	if u.Version() != 2 {
		t.Errorf("version = %d, want 2 (as read)", u.Version())
	}
	if err := u.ChangePassword(""); !errors.Is(err, ErrInvalidUser) {
		t.Errorf("empty hash: %v, want ErrInvalidUser", err)
	}
	if u.PasswordHash() != "new" {
		t.Error("a refused change still changed the hash")
	}
}

func TestUser_Profile(t *testing.T) {
	t.Parallel()

	u, _ := RestoreUser(UserRecord{ID: userID, Name: "Ana", Email: "ana@ex.com", PasswordHash: "h", Version: 1})
	if p := u.Profile(); p.Name.String() != "Ana" || p.Email.String() != "ana@ex.com" {
		t.Errorf("Profile() = %+v", p)
	}
}
