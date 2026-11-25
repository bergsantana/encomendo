package shared

import (
	"fmt"
	"math"
	"strconv"
)

// Money errors. Each matches ErrInvalid.
var (
	// ErrInvalidCurrency means the currency is not a three-letter upper-case
	// ISO 4217 code.
	ErrInvalidCurrency = fmt.Errorf("money: invalid currency: %w", ErrInvalid)
	// ErrCurrencyMismatch means two amounts in different currencies were
	// combined. Encomendo never converts currencies.
	ErrCurrencyMismatch = fmt.Errorf("money: currency mismatch: %w", ErrInvalid)
	// ErrMoneyOverflow means a result does not fit in int64 cents.
	ErrMoneyOverflow = fmt.Errorf("money: amount out of range: %w", ErrInvalid)
)

// Money is an amount in integer cents of one currency. Encomendo never uses
// floats for money (CLAUDE.md, Clean code), and only the backend calculates
// prices and deposits; clients display them.
type Money struct {
	cents    int64
	currency string
}

// NewMoney returns cents of currency. It returns ErrInvalidCurrency unless
// currency is three upper-case ASCII letters, such as BRL.
func NewMoney(cents int64, currency string) (Money, error) {
	if !validCurrency(currency) {
		return Money{}, ErrInvalidCurrency
	}
	return Money{cents: cents, currency: currency}, nil
}

// ZeroMoney returns zero of currency; see NewMoney for the error.
func ZeroMoney(currency string) (Money, error) { return NewMoney(0, currency) }

func validCurrency(c string) bool {
	if len(c) != 3 {
		return false
	}
	for i := range len(c) {
		if c[i] < 'A' || c[i] > 'Z' {
			return false
		}
	}
	return true
}

// Cents returns the amount in cents.
func (m Money) Cents() int64 { return m.cents }

// Currency returns the ISO 4217 code.
func (m Money) Currency() string { return m.currency }

// IsZero reports whether the amount is zero cents.
func (m Money) IsZero() bool { return m.cents == 0 }

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m.cents < 0 }

// Add returns m + o. It returns ErrCurrencyMismatch when the currencies
// differ and ErrMoneyOverflow when the sum does not fit in int64 cents.
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (o.cents > 0 && m.cents > math.MaxInt64-o.cents) || (o.cents < 0 && m.cents < math.MinInt64-o.cents) {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: m.cents + o.cents, currency: m.currency}, nil
}

// Sub returns m − o, with the same errors as Add.
func (m Money) Sub(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (o.cents < 0 && m.cents > math.MaxInt64+o.cents) || (o.cents > 0 && m.cents < math.MinInt64+o.cents) {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: m.cents - o.cents, currency: m.currency}, nil
}

// String formats the amount for logs and test failures, such as "BRL 12.34".
// It is not the pt-BR display format; clients format money for people.
func (m Money) String() string {
	sign := ""
	abs := uint64(m.cents) //nolint:gosec // two's complement: negated below, so MinInt64 keeps its magnitude
	if m.cents < 0 {
		sign = "-"
		abs = -abs
	}
	frac := strconv.FormatUint(abs%100, 10)
	if len(frac) == 1 {
		frac = "0" + frac
	}
	return m.currency + " " + sign + strconv.FormatUint(abs/100, 10) + "." + frac
}
