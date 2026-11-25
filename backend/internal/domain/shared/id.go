package shared

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// ErrInvalidID means a string is not a canonical, non-nil UUID. It matches
// ErrInvalid, so a malformed id in a request is a 400.
var ErrInvalidID = fmt.Errorf("invalid id: %w", ErrInvalid)

// ID is a 128-bit identifier written as a canonical UUID. Encomendo generates
// UUIDv7 (time-ordered) ids in Go, never in the database; the typed IDs below
// share this representation so that one context's id cannot be passed where
// another's is expected.
type ID [16]byte

// ParseID reads the canonical 8-4-4-4-12 hex form, in either case. It
// returns ErrInvalidID for any other form and for the nil UUID, which is
// never a real id.
func ParseID(s string) (ID, error) {
	var id ID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return ID{}, ErrInvalidID
	}
	hexDigits := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.Decode(id[:], []byte(hexDigits)); err != nil {
		return ID{}, ErrInvalidID
	}
	if id.IsZero() {
		return ID{}, ErrInvalidID
	}
	return id, nil
}

// NewV7 builds a UUIDv7 from a Unix time in milliseconds and ten random
// bytes. Only the low 48 bits of unixMilli are used (enough until the year
// 10889), and 74 of the 80 random bits survive the version and variant bits.
// It is pure so that both the real generator and the test fakes produce
// valid, time-ordered ids.
func NewV7(unixMilli int64, random [10]byte) ID {
	var id ID
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(unixMilli)) //nolint:gosec // only the low 48 bits are kept, by design of UUIDv7
	copy(id[0:6], ms[2:8])
	id[6] = 0x70 | random[0]&0x0f // version 7
	id[7] = random[1]
	id[8] = 0x80 | random[2]&0x3f // RFC 9562 variant
	copy(id[9:], random[3:])
	return id
}

// String returns the lowercase canonical form.
func (id ID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], id[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], id[10:16])
	return string(b[:])
}

// IsZero reports whether id is the nil UUID, the value of an unset id.
func (id ID) IsZero() bool { return id == ID{} }

// Version returns the UUID version number (7 for generated ids).
func (id ID) Version() int { return int(id[6] >> 4) }

// MarshalText writes the canonical form, or nothing for the unset id, so
// ids stay strings in the JSON of events and cached views (ADR 0009).
func (id ID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText reads what MarshalText writes. It returns ErrInvalidID for
// anything ParseID refuses, other than the empty string.
func (id *ID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

func parseTyped[T ~[16]byte](s string) (T, error) {
	id, err := ParseID(s)
	if err != nil {
		return T{}, err
	}
	return T(id), nil
}

func marshalTyped[T ~[16]byte](id T) ([]byte, error) {
	if ID(id).IsZero() {
		return []byte{}, nil
	}
	return []byte(ID(id).String()), nil
}

func unmarshalTyped[T ~[16]byte](dst *T, b []byte) error {
	if len(b) == 0 {
		*dst = T{}
		return nil
	}
	id, err := parseTyped[T](string(b))
	if err != nil {
		return err
	}
	*dst = id
	return nil
}

// TenantID identifies a tenant (one store).
type TenantID ID

// ParseTenantID parses a tenant id; see ParseID for the errors.
func ParseTenantID(s string) (TenantID, error) { return parseTyped[TenantID](s) }

// String returns the canonical form.
func (id TenantID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id TenantID) IsZero() bool { return id == TenantID{} }

// MarshalText works as ID.MarshalText.
func (id TenantID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *TenantID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// UserID identifies a staff user (owner, admin, staff or viewer of any number
// of tenants).
type UserID ID

// ParseUserID parses a user id; see ParseID for the errors.
func ParseUserID(s string) (UserID, error) { return parseTyped[UserID](s) }

// String returns the canonical form.
func (id UserID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id UserID) IsZero() bool { return id == UserID{} }

// MarshalText works as ID.MarshalText.
func (id UserID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *UserID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// ClientAccountID identifies a global client account (Minhas encomendas).
type ClientAccountID ID

// ParseClientAccountID parses a client account id; see ParseID for the errors.
func ParseClientAccountID(s string) (ClientAccountID, error) {
	return parseTyped[ClientAccountID](s)
}

// String returns the canonical form.
func (id ClientAccountID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id ClientAccountID) IsZero() bool { return id == ClientAccountID{} }

// MarshalText works as ID.MarshalText.
func (id ClientAccountID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *ClientAccountID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// InvitationID identifies an invitation to join a tenant's team.
type InvitationID ID

// ParseInvitationID parses an invitation id; see ParseID for the errors.
func ParseInvitationID(s string) (InvitationID, error) { return parseTyped[InvitationID](s) }

// String returns the canonical form.
func (id InvitationID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id InvitationID) IsZero() bool { return id == InvitationID{} }

// MarshalText works as ID.MarshalText.
func (id InvitationID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *InvitationID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// WorkflowID identifies a status workflow. Together with a version number it
// names one immutable published workflow (ADR 0004).
type WorkflowID ID

// ParseWorkflowID parses a workflow id; see ParseID for the errors.
func ParseWorkflowID(s string) (WorkflowID, error) { return parseTyped[WorkflowID](s) }

// String returns the canonical form.
func (id WorkflowID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id WorkflowID) IsZero() bool { return id == WorkflowID{} }

// MarshalText works as ID.MarshalText.
func (id WorkflowID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *WorkflowID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }
