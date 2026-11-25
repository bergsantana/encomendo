package shared

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func brl(t *testing.T, cents int64) Money {
	t.Helper()
	m, err := NewMoney(cents, "BRL")
	if err != nil {
		t.Fatalf("NewMoney(%d, BRL) error = %v", cents, err)
	}
	return m
}

func TestNewMoney_ValidatesCurrency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		currency string
		wantErr  bool
	}{
		{"BRL", false},
		{"USD", false},
		{"brl", true}, // ISO 4217 codes are upper case; no silent fixing
		{"BR", true},
		{"BRLL", true},
		{"", true},
		{"B1L", true},
		{"ÇÃO", true},
	}
	for _, tt := range tests {
		t.Run(tt.currency, func(t *testing.T) {
			t.Parallel()

			m, err := NewMoney(150, tt.currency)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidCurrency) || !errors.Is(err, ErrInvalid) {
					t.Errorf("error = %v, want ErrInvalidCurrency wrapping ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if m.Cents() != 150 || m.Currency() != tt.currency {
				t.Errorf("got %d %s", m.Cents(), m.Currency())
			}
		})
	}
}

func TestMoney_AddAndSub(t *testing.T) {
	t.Parallel()

	a, b := brl(t, 1050), brl(t, 299)

	sum, err := a.Add(b)
	if err != nil || sum.Cents() != 1349 || sum.Currency() != "BRL" {
		t.Errorf("Add = %v, %v; want BRL 1349 cents", sum, err)
	}
	diff, err := b.Sub(a)
	if err != nil || diff.Cents() != -751 {
		t.Errorf("Sub = %v, %v; want -751 cents", diff, err)
	}
	if !diff.IsNegative() || sum.IsNegative() {
		t.Error("IsNegative is wrong")
	}
}

func TestMoney_RejectsMixedCurrencies(t *testing.T) {
	t.Parallel()

	usd, err := NewMoney(100, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brl(t, 100).Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add error = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := brl(t, 100).Sub(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sub error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMoney_RejectsOverflow(t *testing.T) {
	t.Parallel()

	if _, err := brl(t, math.MaxInt64).Add(brl(t, 1)); !errors.Is(err, ErrMoneyOverflow) {
		t.Errorf("Add error = %v, want ErrMoneyOverflow", err)
	}
	if _, err := brl(t, math.MinInt64).Sub(brl(t, 1)); !errors.Is(err, ErrMoneyOverflow) {
		t.Errorf("Sub error = %v, want ErrMoneyOverflow", err)
	}
}

func TestZeroMoney(t *testing.T) {
	t.Parallel()

	z, err := ZeroMoney("BRL")
	if err != nil || !z.IsZero() || z.Currency() != "BRL" {
		t.Errorf("ZeroMoney = %v, %v", z, err)
	}
	if _, err := ZeroMoney("real"); !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("ZeroMoney(real) error = %v", err)
	}
	if brl(t, 1).IsZero() {
		t.Error("one cent reports IsZero")
	}
}

func TestMoney_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cents int64
		want  string
	}{
		{0, "BRL 0.00"},
		{5, "BRL 0.05"},
		{123456, "BRL 1234.56"},
		{-50, "BRL -0.50"},
		{math.MinInt64, "BRL -92233720368547758.08"},
	}
	for _, tt := range tests {
		if got := brl(t, tt.cents).String(); got != tt.want {
			t.Errorf("String(%d) = %q, want %q", tt.cents, got, tt.want)
		}
	}
}

func TestMoney_RoundTripsThroughJSON(t *testing.T) {
	t.Parallel()

	// Events and cached views carry money as JSON (ADR 0009).
	type view struct {
		Price Money `json:"price"`
		Unset Money `json:"unset"`
	}
	in := view{Price: brl(t, 45000)}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"price":{"cents":45000,"currency":"BRL"},"unset":null}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
	var out view
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestMoney_UnmarshalValidates(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		`{"cents":100,"currency":"real"}`,
		`{"cents":100}`,
		`{"currency":"BRL"}`, // a missing amount is not zero
		`{"cents":"100","currency":"BRL"}`,
		`[]`,
	} {
		var m Money
		if err := json.Unmarshal([]byte(in), &m); err == nil {
			t.Errorf("Unmarshal(%s) accepted %+v", in, m)
		}
	}

	// null is "not present": the value stays as it was.
	m := brl(t, 7)
	if err := m.UnmarshalJSON([]byte("null")); err != nil || m != brl(t, 7) {
		t.Errorf("UnmarshalJSON(null) = %v, %v; want the value unchanged", m, err)
	}
}

func FuzzMoney_AddSubRoundTrip(f *testing.F) {
	f.Add(int64(0), int64(0))
	f.Add(int64(1050), int64(-299))
	f.Add(int64(math.MaxInt64), int64(1))
	f.Add(int64(math.MinInt64), int64(-1))
	f.Fuzz(func(t *testing.T, a, b int64) {
		ma, err := NewMoney(a, "BRL")
		if err != nil {
			t.Fatal(err)
		}
		mb, err := NewMoney(b, "BRL")
		if err != nil {
			t.Fatal(err)
		}
		sum, err := ma.Add(mb)
		if err != nil {
			if !errors.Is(err, ErrMoneyOverflow) {
				t.Fatalf("Add(%d, %d) error = %v", a, b, err)
			}
			return
		}
		back, err := sum.Sub(mb)
		if err != nil || back != ma {
			t.Fatalf("(%d + %d) - %d = %v, %v; want %d", a, b, b, back, err, a)
		}
	})
}
