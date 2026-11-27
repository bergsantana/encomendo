package shared

import (
	"fmt"
	"time"
)

// ErrInvalidDate means a civil date is not a real calendar day in the
// supported years, or a string is not written as YYYY-MM-DD. It matches
// ErrInvalid, so a malformed date in a request is a 400.
var ErrInvalidDate = fmt.Errorf("invalid date: %w", ErrInvalid)

// The supported years. Orders and deadlines live in the present; the bounds
// refuse typos such as 0026 before they reach the database.
const (
	minDateYear = 2000
	maxDateYear = 2200
)

// CivilDate is a calendar day with no time of day and no zone. Deadlines count
// calendar days in the store's time zone (ADR 0030, Dates), so a date never
// shifts with the server's zone. The zero value is unset. Dates compare with
// ==.
type CivilDate struct {
	y int
	m time.Month
	d int
}

// NewCivilDate returns the day y-m-d. It returns ErrInvalidDate for a day that
// does not exist (2026-02-30) and for years outside 2000..2200.
func NewCivilDate(y int, m time.Month, d int) (CivilDate, error) {
	if y < minDateYear || y > maxDateYear || m < time.January || m > time.December || d < 1 || d > daysIn(y, m) {
		return CivilDate{}, ErrInvalidDate
	}
	return CivilDate{y: y, m: m, d: d}, nil
}

// ParseCivilDate reads exactly YYYY-MM-DD: four digits, a dash, two digits, a
// dash and two digits, with no spaces, signs or time. It returns
// ErrInvalidDate for anything else and for days NewCivilDate refuses.
func ParseCivilDate(s string) (CivilDate, error) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return CivilDate{}, ErrInvalidDate
	}
	y, okY := digits(s[0:4])
	m, okM := digits(s[5:7])
	d, okD := digits(s[8:10])
	if !okY || !okM || !okD {
		return CivilDate{}, ErrInvalidDate
	}
	return NewCivilDate(y, time.Month(m), d)
}

// DateOf returns the calendar day that t falls on in loc, which is how a
// store's "today" is found: at 03:30 UTC it is already the same day in Sao
// Paulo (UTC-3) but still the day before in Manaus (UTC-4). A nil loc means
// UTC. The clock is the caller's, never read here.
func DateOf(t time.Time, loc *time.Location) CivilDate {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	return CivilDate{y: y, m: m, d: d}
}

// AddDays returns the date n calendar days later, or earlier when n is
// negative. An unset date stays unset.
func (d CivilDate) AddDays(n int) CivilDate {
	if d.IsZero() {
		return d
	}
	y, m, day := d.utc().AddDate(0, 0, n).Date()
	return CivilDate{y: y, m: m, d: day}
}

// DaysUntil returns o minus d in calendar days, negative when o is earlier.
// It returns 0 when either date is unset.
func (d CivilDate) DaysUntil(o CivilDate) int {
	if d.IsZero() || o.IsZero() {
		return 0
	}
	return int(o.utc().Sub(d.utc()) / (24 * time.Hour))
}

// Before reports whether d is a day earlier than o. An unset date is before
// every set one.
func (d CivilDate) Before(o CivilDate) bool {
	if d.y != o.y {
		return d.y < o.y
	}
	if d.m != o.m {
		return d.m < o.m
	}
	return d.d < o.d
}

// After reports whether d is a day later than o.
func (d CivilDate) After(o CivilDate) bool { return o.Before(d) }

// IsZero reports whether d is unset.
func (d CivilDate) IsZero() bool { return d == CivilDate{} }

// String returns YYYY-MM-DD, or "" for an unset date.
func (d CivilDate) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.y, int(d.m), d.d)
}

// MarshalText writes YYYY-MM-DD, or nothing for an unset date, so dates stay
// strings in JSON.
func (d CivilDate) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText reads what MarshalText writes: the empty string is the unset
// date, anything else must satisfy ParseCivilDate.
func (d *CivilDate) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*d = CivilDate{}
		return nil
	}
	parsed, err := ParseCivilDate(string(b))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Latest returns the latest of ds, or the unset date when ds is empty.
func Latest(ds ...CivilDate) CivilDate {
	var latest CivilDate
	for _, d := range ds {
		if d.After(latest) {
			latest = d
		}
	}
	return latest
}

// utc is the date at midnight UTC, used only for day arithmetic: UTC has no
// daylight saving, so adding days never lands on a skipped or repeated hour.
func (d CivilDate) utc() time.Time {
	return time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC)
}

func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func digits(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}
