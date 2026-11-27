package shared

import (
	"errors"
	"testing"
	"time"
	_ "time/tzdata" // the test machine may have no zone files
)

func mustDate(t testing.TB, s string) CivilDate {
	t.Helper()
	d, err := ParseCivilDate(s)
	if err != nil {
		t.Fatalf("ParseCivilDate(%q): %v", s, err)
	}
	return d
}

func TestNewCivilDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		y       int
		m       time.Month
		d       int
		wantErr bool
	}{
		{"ordinary day", 2026, time.December, 1, false},
		{"leap day", 2028, time.February, 29, false},
		{"february 30", 2026, time.February, 30, true},
		{"february 29 in a common year", 2026, time.February, 29, true},
		{"day zero", 2026, time.March, 0, true},
		{"day 32", 2026, time.March, 32, true},
		{"month zero", 2026, 0, 1, true},
		{"month 13", 2026, 13, 1, true},
		{"first supported year", 2000, time.January, 1, false},
		{"last supported year", 2200, time.December, 31, false},
		{"year before range", 1999, time.December, 31, true},
		{"year after range", 2201, time.January, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewCivilDate(tt.y, tt.m, tt.d)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidDate) || !errors.Is(err, ErrInvalid) || !got.IsZero() {
					t.Errorf("NewCivilDate = %v, %v; want zero and ErrInvalidDate", got, err)
				}
				return
			}
			if err != nil || got.IsZero() {
				t.Errorf("NewCivilDate = %v, %v; want a date", got, err)
			}
		})
	}
}

func TestParseCivilDate_IsStrict(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"", "2026-12-1", "2026-1-01", "26-12-01", "2026/12/01", "2026-12-01 ",
		" 2026-12-01", "2026-12-01T00:00:00Z", "+026-12-01", "2026-12-0a",
		"2026-02-30", "1999-12-31", "2201-01-01", "2026-13-01", "2026-00-10", "2026-12-00",
	} {
		if got, err := ParseCivilDate(s); !errors.Is(err, ErrInvalidDate) || !got.IsZero() {
			t.Errorf("ParseCivilDate(%q) = %v, %v; want ErrInvalidDate", s, got, err)
		}
	}
	if got := mustDate(t, "2026-12-01"); got.String() != "2026-12-01" {
		t.Errorf("round trip = %q", got)
	}
}

func TestCivilDate_AddDays_CrossesMonthAndYear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from string
		n    int
		want string
	}{
		{"2026-12-01", 20, "2026-12-21"},
		{"2026-12-31", 1, "2027-01-01"},
		{"2027-01-01", -1, "2026-12-31"},
		{"2026-01-31", 1, "2026-02-01"},
		{"2028-02-28", 1, "2028-02-29"},
		{"2028-02-28", 2, "2028-03-01"},
		{"2026-02-28", 1, "2026-03-01"},
		{"2026-12-15", 0, "2026-12-15"},
		{"2026-12-15", 365, "2027-12-15"},
		{"2026-12-15", -365, "2025-12-15"},
	}
	for _, tt := range tests {
		if got := mustDate(t, tt.from).AddDays(tt.n); got.String() != tt.want {
			t.Errorf("%s + %d = %s, want %s", tt.from, tt.n, got, tt.want)
		}
	}
	if got := (CivilDate{}).AddDays(5); !got.IsZero() {
		t.Errorf("unset + 5 = %v, want unset", got)
	}
}

func TestCivilDate_DaysUntil(t *testing.T) {
	t.Parallel()

	a, b := mustDate(t, "2026-12-01"), mustDate(t, "2027-01-05")
	if got := a.DaysUntil(b); got != 35 {
		t.Errorf("DaysUntil = %d, want 35", got)
	}
	if got := b.DaysUntil(a); got != -35 {
		t.Errorf("DaysUntil = %d, want -35", got)
	}
	if got := a.DaysUntil(a); got != 0 {
		t.Errorf("DaysUntil self = %d, want 0", got)
	}
	if got := (CivilDate{}).DaysUntil(a); got != 0 {
		t.Errorf("unset.DaysUntil = %d, want 0", got)
	}
	if got := a.DaysUntil(CivilDate{}); got != 0 {
		t.Errorf("DaysUntil(unset) = %d, want 0", got)
	}
}

func TestCivilDate_Ordering(t *testing.T) {
	t.Parallel()

	early, late := mustDate(t, "2026-12-01"), mustDate(t, "2027-01-01")
	sameYear := mustDate(t, "2026-12-02")
	sameMonthLater := mustDate(t, "2026-12-20")
	laterMonth := mustDate(t, "2026-11-30")
	for _, tt := range []struct {
		name          string
		a, b          CivilDate
		before, after bool
	}{
		{"year decides", early, late, true, false},
		{"year decides, reversed", late, early, false, true},
		{"month decides", laterMonth, early, true, false},
		{"day decides", sameYear, sameMonthLater, true, false},
		{"equal", early, early, false, false},
		{"unset before set", CivilDate{}, early, true, false},
	} {
		if got := tt.a.Before(tt.b); got != tt.before {
			t.Errorf("%s: Before = %v, want %v", tt.name, got, tt.before)
		}
		if got := tt.a.After(tt.b); got != tt.after {
			t.Errorf("%s: After = %v, want %v", tt.name, got, tt.after)
		}
	}
}

func TestDateOf_ManausAndSaoPauloDifferAt0330UTC(t *testing.T) {
	t.Parallel()

	manaus, err := time.LoadLocation("America/Manaus")
	if err != nil {
		t.Fatal(err)
	}
	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, time.December, 1, 3, 30, 0, 0, time.UTC)

	if got := DateOf(instant, manaus).String(); got != "2026-11-30" {
		t.Errorf("Manaus = %s, want 2026-11-30", got)
	}
	if got := DateOf(instant, saoPaulo).String(); got != "2026-12-01" {
		t.Errorf("Sao Paulo = %s, want 2026-12-01", got)
	}
	if got := DateOf(instant, nil).String(); got != "2026-12-01" {
		t.Errorf("nil loc = %s, want the UTC day 2026-12-01", got)
	}
}

func TestCivilDate_Text(t *testing.T) {
	t.Parallel()

	d := mustDate(t, "2026-12-21")
	b, err := d.MarshalText()
	if err != nil || string(b) != "2026-12-21" {
		t.Fatalf("MarshalText = %q, %v", b, err)
	}
	var back CivilDate
	if err := back.UnmarshalText(b); err != nil || back != d {
		t.Errorf("UnmarshalText = %v, %v; want %v", back, err, d)
	}

	if b, err := (CivilDate{}).MarshalText(); err != nil || len(b) != 0 {
		t.Errorf("unset MarshalText = %q, %v; want empty", b, err)
	}
	back = d
	if err := back.UnmarshalText(nil); err != nil || !back.IsZero() {
		t.Errorf("UnmarshalText(empty) = %v, %v; want unset", back, err)
	}
	if err := back.UnmarshalText([]byte("2026-02-30")); !errors.Is(err, ErrInvalidDate) {
		t.Errorf("UnmarshalText(bad) = %v, want ErrInvalidDate", err)
	}
	if got := (CivilDate{}).String(); got != "" {
		t.Errorf("unset String = %q, want empty", got)
	}
}

func TestLatest(t *testing.T) {
	t.Parallel()

	a, b, c := mustDate(t, "2026-12-01"), mustDate(t, "2026-12-20"), mustDate(t, "2027-01-02")
	if got := Latest(); !got.IsZero() {
		t.Errorf("Latest() = %v, want unset", got)
	}
	if got := Latest(a); got != a {
		t.Errorf("Latest(a) = %v, want %v", got, a)
	}
	if got := Latest(b, c, a); got != c {
		t.Errorf("Latest(b, c, a) = %v, want %v", got, c)
	}
	if got := Latest(CivilDate{}, a); got != a {
		t.Errorf("Latest(unset, a) = %v, want %v", got, a)
	}
}

func FuzzParseCivilDate(f *testing.F) {
	f.Add("2026-12-01")
	f.Add("2028-02-29")
	f.Add("2026-02-30")
	f.Add("")
	f.Add("2026-12-01T00:00:00Z")
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseCivilDate(s)
		if err != nil {
			if !errors.Is(err, ErrInvalidDate) || !d.IsZero() {
				t.Fatalf("ParseCivilDate(%q) = %v, %v", s, d, err)
			}
			return
		}
		// Accepted input is exactly the canonical form, and survives a round trip.
		if d.String() != s {
			t.Fatalf("ParseCivilDate(%q).String() = %q", s, d)
		}
		if next := d.AddDays(1).AddDays(-1); next != d {
			t.Fatalf("%s + 1 - 1 = %s", d, next)
		}
		if n := d.DaysUntil(d.AddDays(37)); n != 37 {
			t.Fatalf("%s.DaysUntil(+37) = %d", d, n)
		}
	})
}
