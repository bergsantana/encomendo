package orderports

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
)

var numberCases = []contractCase{
	{"count up from 1 in each store", numbersPerStore},
	{"a failed transaction gives its number back", numbersRolledBack},
	{"concurrent creates get different numbers", numbersConcurrent},
}

func (h Harness) next(t *testing.T, s store) order.Number {
	t.Helper()
	var n order.Number
	h.in(t, s, func(ctx context.Context) error {
		var err error
		n, err = h.Orders.NextNumber(ctx, s.id)
		return err
	})
	return n
}

func numbersPerStore(t *testing.T, h Harness) {
	a, b := h.newStore(t), h.newStore(t)
	got := []order.Number{h.next(t, a), h.next(t, a), h.next(t, b), h.next(t, a), h.next(t, b)}
	if want := []order.Number{1, 2, 1, 3, 2}; !slices.Equal(got, want) {
		t.Errorf("numbers = %v, want %v", got, want)
	}
}

func numbersRolledBack(t *testing.T, h Harness) {
	s := h.newStore(t)
	boom := errors.New("boom")
	err := h.try(s.scope(), func(ctx context.Context) error {
		if _, err := h.Orders.NextNumber(ctx, s.id); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if n := h.next(t, s); n != 1 {
		t.Errorf("number after a rolled back one = %d, want 1", n)
	}
}

// numbersConcurrent has several creators take a number and store an order
// at once: every one commits, and no two share a number (the unique key on
// the store's numbers would refuse a repeat).
func numbersConcurrent(t *testing.T, h Harness) {
	s := h.newStore(t)
	const creators = 6
	var wg sync.WaitGroup
	numbers := make([]order.Number, creators)
	errs := make([]error, creators)
	for i := range creators {
		wg.Go(func() {
			errs[i] = h.try(s.scope(), func(ctx context.Context) error {
				n, err := h.Orders.NextNumber(ctx, s.id)
				if err != nil {
					return err
				}
				numbers[i] = n
				return h.Orders.Insert(ctx, h.newOrder(t, s, n))
			})
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("a creator failed: %v", err)
	}
	slices.Sort(numbers)
	for i, n := range numbers {
		if n != order.Number(i+1) {
			t.Fatalf("numbers = %v, want 1 to %d, each once", numbers, creators)
		}
	}
}
