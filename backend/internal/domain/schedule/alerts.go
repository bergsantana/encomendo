package schedule

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Alert is a warning about an item's schedule. It sits on top of the order's
// status and is never a status.
type Alert string

const (
	// AlertAtRisk is "Pode atrasar": the item is expected after the arrival
	// limit. It is internal: the client never sees it (see Alerts).
	AlertAtRisk Alert = "at_risk"
	// AlertLate is "Atrasada": the item's expected date has passed and it has
	// not arrived.
	AlertLate Alert = "late"
)

// ItemState is what the alerts need to know about one order item.
type ItemState struct {
	// Expected is the item's expected date, from ExpectedDate.
	Expected shared.CivilDate
	// Arrived is true once the item is received, ready or installed.
	Arrived bool
}

// Alerts returns the alerts of one item, at_risk first. An item that has
// arrived has none. at_risk applies when Expected is after limit, and late
// when Expected is before today, so an item can carry both. An unset
// Expected (no forecast yet) has none, and an unset limit (no promise yet)
// cannot put an item at risk. today is the day in the store's time zone
// (shared.DateOf), never read from a clock here.
//
// Product rule: "Pode atrasar" and "Atrasada" are alerts computed on read,
// not statuses (docs/architecture.md, section 4.2, Deadline), and "Pode
// atrasar" is never shown to the client (docs/product/design-document.md,
// System rules).
func Alerts(s ItemState, limit, today shared.CivilDate) []Alert {
	if s.Arrived || s.Expected.IsZero() {
		return nil
	}
	var out []Alert
	if !limit.IsZero() && s.Expected.After(limit) {
		out = append(out, AlertAtRisk)
	}
	if s.Expected.Before(today) {
		out = append(out, AlertLate)
	}
	return out
}
