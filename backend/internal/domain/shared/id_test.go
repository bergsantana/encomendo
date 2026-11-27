package shared

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseID_AcceptsCanonicalForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"lowercase", "0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e", "0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e"},
		{"uppercase is normalised", "0190F3C4-5E1A-7B2C-8D3E-4F5A6B7C8D9E", "0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e"},
		{"any version", "550e8400-e29b-41d4-a716-446655440000", "550e8400-e29b-41d4-a716-446655440000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, err := ParseID(tt.in)
			if err != nil {
				t.Fatalf("ParseID(%q) error = %v", tt.in, err)
			}
			if got := id.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseID_RejectsEverythingElse(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"",
		"00000000-0000-0000-0000-000000000000", // the nil UUID is never a real id
		"0190f3c45e1a7b2c8d3e4f5a6b7c8d9e",     // no hyphens
		"{0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e}",
		"urn:uuid:0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e",
		"0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9",   // short
		"0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e0", // long
		"0190f3c4_5e1a-7b2c-8d3e-4f5a6b7c8d9e",  // wrong separator
		"0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9g",  // not hex
		" 0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9",
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			_, err := ParseID(in)
			if !errors.Is(err, ErrInvalidID) || !errors.Is(err, ErrInvalid) {
				t.Errorf("ParseID(%q) error = %v, want ErrInvalidID wrapping ErrInvalid", in, err)
			}
		})
	}
}

func TestNewV7_LaysOutTimeVersionAndVariant(t *testing.T) {
	t.Parallel()

	random := [10]byte{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88, 0x77, 0x66}
	id := NewV7(0x0190f3c45e1a, random)

	if got, want := id.String(), "0190f3c4-5e1a-7fee-9dcc-bbaa99887766"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if id.Version() != 7 {
		t.Errorf("Version() = %d, want 7", id.Version())
	}
	if id[8]&0xc0 != 0x80 {
		t.Errorf("variant bits = %08b, want 10xxxxxx", id[8])
	}
}

func TestNewV7_SortsByTime(t *testing.T) {
	t.Parallel()

	high := [10]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	earlier := NewV7(1_700_000_000_000, high)
	later := NewV7(1_700_000_000_001, [10]byte{})

	if earlier.String() >= later.String() {
		t.Errorf("%s should sort before %s", earlier, later)
	}
}

func TestNewV7_IsNeverZero(t *testing.T) {
	t.Parallel()

	if NewV7(0, [10]byte{}).IsZero() {
		t.Error("NewV7 returned the nil UUID; version bits must always be set")
	}
}

func TestID_IsZero(t *testing.T) {
	t.Parallel()

	if !(ID{}).IsZero() {
		t.Error("ID{}.IsZero() = false")
	}
	if NewV7(1, [10]byte{}).IsZero() {
		t.Error("a generated id reports IsZero")
	}
}

// typedID is what every typed ID in this package offers.
type typedID interface {
	comparable
	String() string
	IsZero() bool
}

func checkTypedID[T typedID](t *testing.T, parse func(string) (T, error)) {
	t.Helper()
	const s = "0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e"

	id, err := parse(s)
	if err != nil {
		t.Fatalf("parse(%q) error = %v", s, err)
	}
	if id.String() != s || id.IsZero() {
		t.Errorf("parsed id = %q (zero %v), want %q", id.String(), id.IsZero(), s)
	}
	var zero T
	if !zero.IsZero() {
		t.Error("zero value reports IsZero() = false")
	}
	if _, err := parse("nope"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("parse(nope) error = %v, want ErrInvalidID", err)
	}
}

func TestTypedIDs_ParseFormatAndZero(t *testing.T) {
	t.Parallel()

	t.Run("TenantID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseTenantID) })
	t.Run("UserID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseUserID) })
	t.Run("ClientAccountID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseClientAccountID) })
	t.Run("InvitationID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseInvitationID) })
	t.Run("WorkflowID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseWorkflowID) })
	t.Run("SupplierID", func(t *testing.T) { t.Parallel(); checkTypedID(t, ParseSupplierID) })
}

func TestTypedIDs_RoundTripThroughJSON(t *testing.T) {
	t.Parallel()

	// Events and cached views carry ids as JSON (ADR 0009): they must come
	// back unchanged, and an unset id must stay unset.
	type payload struct {
		Tenant  TenantID        `json:"tenant"`
		User    UserID          `json:"user"`
		Account ClientAccountID `json:"account"`
		Invite  InvitationID    `json:"invite"`
		Flow    WorkflowID      `json:"flow"`
		Maker   SupplierID      `json:"maker"`
		Raw     ID              `json:"raw"`
	}
	in := payload{
		Tenant: TenantID(NewV7(1, [10]byte{1})),
		User:   UserID(NewV7(2, [10]byte{2})),
		Invite: InvitationID(NewV7(3, [10]byte{3})),
		Flow:   WorkflowID(NewV7(4, [10]byte{4})),
		Maker:  SupplierID(NewV7(6, [10]byte{6})),
		Raw:    NewV7(5, [10]byte{5}),
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tenant":"`+in.Tenant.String()+`"`) || !strings.Contains(string(data), `"account":""`) {
		t.Errorf("JSON = %s, want canonical strings and an empty string for the unset id", data)
	}
	var out payload
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestTypedIDs_UnmarshalRejectsMalformed(t *testing.T) {
	t.Parallel()

	targets := map[string]any{
		"TenantID": new(TenantID), "UserID": new(UserID), "ClientAccountID": new(ClientAccountID),
		"InvitationID": new(InvitationID), "WorkflowID": new(WorkflowID), "SupplierID": new(SupplierID), "ID": new(ID),
	}
	for name, dst := range targets {
		if err := json.Unmarshal([]byte(`"not-an-id"`), dst); !errors.Is(err, ErrInvalidID) {
			t.Errorf("%s: error = %v, want ErrInvalidID", name, err)
		}
		// Unset is the empty string, so the nil UUID is still refused.
		if err := json.Unmarshal([]byte(`"00000000-0000-0000-0000-000000000000"`), dst); !errors.Is(err, ErrInvalidID) {
			t.Errorf("%s: nil UUID error = %v, want ErrInvalidID", name, err)
		}
	}
}

func FuzzParseID(f *testing.F) {
	for _, seed := range []string{
		"0190f3c4-5e1a-7b2c-8d3e-4f5a6b7c8d9e",
		"00000000-0000-0000-0000-000000000000",
		"",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		id, err := ParseID(s)
		if err != nil {
			return
		}
		// Anything accepted round-trips to its lowercase form and is not nil.
		if id.IsZero() {
			t.Fatalf("ParseID(%q) accepted the nil UUID", s)
		}
		if got := id.String(); got != strings.ToLower(s) {
			t.Fatalf("ParseID(%q).String() = %q", s, got)
		}
	})
}
