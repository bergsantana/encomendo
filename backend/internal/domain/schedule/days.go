package schedule

import "errors"

// ErrNegativeDays means a number of days below zero was given to Days. Day
// counts come from validated catalog and settings values (0 to 365), so this
// is a bug in the caller, not bad input: it matches no shared sentinel, so
// the API answers 500, not 400.
var ErrNegativeDays = errors.New("schedule: days cannot be negative")

// Days returns the days an order or kit needs: the slowest of the
// items' ready days, plus assembly days, plus the safety margin. With no
// items it returns assembly + margin. It returns 0 and ErrNegativeDays when
// any argument is below zero.
//
// Product rule: delivery = latest item ready date + assembly days + safety
// margin (docs/product/design-document.md, Deadline formula). The Kit
// Scrambler CB 500X, whose slowest part takes 15 days, with 3 days of
// assembly and a margin of 2, is ready in 20.
func Days(ready []int, assembly, margin int) (int, error) {
	if assembly < 0 || margin < 0 {
		return 0, ErrNegativeDays
	}
	slowest := 0
	for _, d := range ready {
		if d < 0 {
			return 0, ErrNegativeDays
		}
		slowest = max(slowest, d)
	}
	return slowest + assembly + margin, nil
}
