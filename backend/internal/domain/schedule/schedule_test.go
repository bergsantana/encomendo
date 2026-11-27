package schedule

import (
	"slices"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func date(t testing.TB, s string) shared.CivilDate {
	t.Helper()
	d, err := shared.ParseCivilDate(s)
	if err != nil {
		t.Fatalf("ParseCivilDate(%q): %v", s, err)
	}
	return d
}

func TestDelivery_KitScramblerCB500X(t *testing.T) {
	t.Parallel()

	// docs/product/design-document.md, Deadline formula: the leather seat
	// takes 15 days, the slowest part; 15 + 3 + 2 = day 20, and the arrival
	// limit is 20 - 3 = day 17.
	release := date(t, "2026-12-01")
	expected := []shared.CivilDate{
		ExpectedDate(Timing{LeadDays: 5}, release),
		ExpectedDate(Timing{LeadDays: 15}, release),
		ExpectedDate(Timing{LeadDays: 7}, release),
	}
	promise := Delivery(expected, release, 3, 2)
	if got := promise.String(); got != "2026-12-21" {
		t.Errorf("Delivery = %s, want 2026-12-21", got)
	}
	if got := ArrivalLimit(promise, 3).String(); got != "2026-12-18" {
		t.Errorf("ArrivalLimit = %s, want 2026-12-18", got)
	}
	if got := release.DaysUntil(promise); got != 20 {
		t.Errorf("promise is day %d, want 20", got)
	}
	if got := release.DaysUntil(ArrivalLimit(promise, 3)); got != 17 {
		t.Errorf("arrival limit is day %d, want 17", got)
	}
}

func TestDelivery(t *testing.T) {
	t.Parallel()

	release := date(t, "2026-12-01")
	tests := []struct {
		name     string
		expected []shared.CivilDate
		release  shared.CivilDate
		assembly int
		margin   int
		want     string
	}{
		{"no items is assembly and margin from release", nil, release, 3, 2, "2026-12-06"},
		{"an item ready before release does not pull it earlier", []shared.CivilDate{date(t, "2026-11-20")}, release, 3, 2, "2026-12-06"},
		{"crosses the year", []shared.CivilDate{date(t, "2026-12-30")}, release, 3, 2, "2027-01-04"},
		{"no assembly phase", []shared.CivilDate{date(t, "2026-12-10")}, release, 0, 2, "2026-12-12"},
		{"negative days count as zero", []shared.CivilDate{date(t, "2026-12-10")}, release, -3, -2, "2026-12-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := slices.Clone(tt.expected)
			if got := Delivery(tt.expected, tt.release, tt.assembly, tt.margin).String(); got != tt.want {
				t.Errorf("Delivery = %s, want %s", got, tt.want)
			}
			if !slices.Equal(before, tt.expected) {
				t.Error("Delivery changed its input")
			}
		})
	}
}

func TestArrivalLimit(t *testing.T) {
	t.Parallel()

	promise := date(t, "2027-01-02")
	if got := ArrivalLimit(promise, 3).String(); got != "2026-12-30" {
		t.Errorf("ArrivalLimit = %s, want 2026-12-30", got)
	}
	if got := ArrivalLimit(promise, 0); got != promise {
		t.Errorf("ArrivalLimit with no assembly = %s, want the promise", got)
	}
	if got := ArrivalLimit(promise, -3); got != promise {
		t.Errorf("ArrivalLimit with negative assembly = %s, want the promise", got)
	}
	if got := ArrivalLimit(shared.CivilDate{}, 3); !got.IsZero() {
		t.Errorf("ArrivalLimit of no promise = %s, want unset", got)
	}
}

func TestExpectedDate_PromisedReplacesLead(t *testing.T) {
	t.Parallel()

	release := date(t, "2026-12-01")
	if got := ExpectedDate(Timing{LeadDays: 15}, release).String(); got != "2026-12-16" {
		t.Errorf("without a promise = %s, want 2026-12-16", got)
	}
	promised := date(t, "2026-12-05")
	if got := ExpectedDate(Timing{LeadDays: 15, Promised: promised}, release); got != promised {
		t.Errorf("with a promise = %s, want %s", got, promised)
	}
	later := date(t, "2027-01-20")
	if got := ExpectedDate(Timing{LeadDays: 15, Promised: later}, release); got != later {
		t.Errorf("a promise later than the lead = %s, want %s", got, later)
	}
}

func TestExpectedDate_AddsOptionDays(t *testing.T) {
	t.Parallel()

	release := date(t, "2026-12-01")
	if got := ExpectedDate(Timing{LeadDays: 5, ExtraDays: 3}, release).String(); got != "2026-12-09" {
		t.Errorf("lead + extra = %s, want 2026-12-09", got)
	}
	promised := date(t, "2026-12-05")
	if got := ExpectedDate(Timing{LeadDays: 5, ExtraDays: 3, Promised: promised}, release).String(); got != "2026-12-08" {
		t.Errorf("promised + extra = %s, want 2026-12-08", got)
	}
	if got := ExpectedDate(Timing{LeadDays: -4, ExtraDays: -2}, release); got != release {
		t.Errorf("negative days = %s, want release", got)
	}
	if got := ExpectedDate(Timing{LeadDays: 5}, shared.CivilDate{}); !got.IsZero() {
		t.Errorf("no release and no promise = %s, want unset", got)
	}
}

func TestAlerts_(t *testing.T) {
	t.Parallel()

	limit := date(t, "2026-12-18")
	today := date(t, "2026-12-10")
	tests := []struct {
		name  string
		state ItemState
		limit shared.CivilDate
		today shared.CivilDate
		want  []Alert
	}{
		{"on time", ItemState{Expected: date(t, "2026-12-15")}, limit, today, nil},
		{"expected on the limit is on time", ItemState{Expected: limit}, limit, today, nil},
		{"expected today is not late", ItemState{Expected: today}, limit, today, nil},
		{"at risk", ItemState{Expected: date(t, "2026-12-19")}, limit, today, []Alert{AlertAtRisk}},
		{"late", ItemState{Expected: date(t, "2026-12-09")}, limit, today, []Alert{AlertLate}},
		{"both", ItemState{Expected: date(t, "2026-12-19")}, limit, date(t, "2026-12-25"), []Alert{AlertAtRisk, AlertLate}},
		{"arrived and past due", ItemState{Expected: date(t, "2026-12-09"), Arrived: true}, limit, today, nil},
		{"arrived and past the limit", ItemState{Expected: date(t, "2026-12-19"), Arrived: true}, limit, today, nil},
		{"no forecast yet", ItemState{}, limit, today, nil},
		{"no promise yet cannot be at risk", ItemState{Expected: date(t, "2027-03-01")}, shared.CivilDate{}, today, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Alerts(tt.state, tt.limit, tt.today); !slices.Equal(got, tt.want) {
				t.Errorf("Alerts = %v, want %v", got, tt.want)
			}
		})
	}
}

func FuzzDelivery_NeverBeforeRelease(f *testing.F) {
	f.Add(2026, 12, 1, 15, 7, 3, 2)
	f.Add(2000, 1, 1, 0, 0, 0, 0)
	f.Add(2200, 12, 31, 365, 365, 365, 365)
	f.Add(2026, 12, 1, -5, 0, -1, -1)
	f.Fuzz(func(t *testing.T, y, m, d, leadA, leadB, assembly, margin int) {
		const bound = 1 << 20 // the contract allows 0..365; this keeps AddDays far from overflow
		for _, v := range []int{leadA, leadB, assembly, margin} {
			if v > bound || v < -bound {
				t.Skip()
			}
		}
		release, err := shared.NewCivilDate(y, time.Month(m), d)
		if err != nil {
			t.Skip()
		}
		expected := []shared.CivilDate{
			ExpectedDate(Timing{LeadDays: leadA}, release),
			ExpectedDate(Timing{LeadDays: leadB, ExtraDays: leadA}, release),
		}
		promise := Delivery(expected, release, assembly, margin)
		if promise.Before(release) {
			t.Fatalf("Delivery %s is before release %s", promise, release)
		}
		for _, e := range expected {
			if promise.Before(e) {
				t.Fatalf("Delivery %s is before an item's expected date %s", promise, e)
			}
		}
		if limit := ArrivalLimit(promise, assembly); limit.After(promise) {
			t.Fatalf("ArrivalLimit %s is after the promise %s", limit, promise)
		}
	})
}
