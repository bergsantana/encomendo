package purchasingports

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
)

var numberCases = []repoCase{
	{"numbers count from 1 in each store, apart", numbersPerStore},
	{"PeekNumber takes nothing", peekTakesNothing},
	{"a rolled back number is given again, a committed one never", rolledBackNumberReturns},
	{"two concurrent sends never share a number", concurrentSendsGetTwoNumbers},
}

func numbersPerStore(t *testing.T, h RepositoryHarness) {
	a, b := h.NewStore(t), h.NewStore(t)
	take := func(s Store) purchasing.Number {
		var n purchasing.Number
		h.in(t, s, func(ctx context.Context) (err error) {
			n, err = h.Repo.NextNumber(ctx, s.Tenant)
			return err
		})
		return n
	}
	got := []purchasing.Number{take(a), take(a), take(b), take(a), take(b)}
	if want := []purchasing.Number{1, 2, 1, 3, 2}; !slices.Equal(got, want) {
		t.Errorf("numbers = %v, want %v", got, want)
	}
}

func peekTakesNothing(t *testing.T, h RepositoryHarness) {
	s := h.NewStore(t)
	h.in(t, s, func(ctx context.Context) error {
		for range 2 {
			if n, err := h.Repo.PeekNumber(ctx, s.Tenant); err != nil || n != 1 {
				t.Errorf("PeekNumber on a new store = %v, %v, want 1", n, err)
			}
		}
		if n, err := h.Repo.NextNumber(ctx, s.Tenant); err != nil || n != 1 {
			t.Errorf("NextNumber after peeks = %v, %v, want 1", n, err)
		}
		if n, err := h.Repo.PeekNumber(ctx, s.Tenant); err != nil || n != 2 {
			t.Errorf("PeekNumber after one number = %v, %v, want 2", n, err)
		}
		return nil
	})
}

func rolledBackNumberReturns(t *testing.T, h RepositoryHarness) {
	s := h.NewStore(t)
	boom := errors.New("boom")
	err := h.try(s.Scope(), func(ctx context.Context) error {
		if n, err := h.Repo.NextNumber(ctx, s.Tenant); err != nil || n != 1 {
			t.Errorf("first NextNumber = %v, %v, want 1", n, err)
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("the transaction answered %v, want the rollback's error", err)
	}
	h.in(t, s, func(ctx context.Context) error {
		if n, err := h.Repo.NextNumber(ctx, s.Tenant); err != nil || n != 1 {
			t.Errorf("NextNumber after a rollback = %v, %v, want 1 again", n, err)
		}
		return nil
	})
	h.in(t, s, func(ctx context.Context) error {
		if n, err := h.Repo.NextNumber(ctx, s.Tenant); err != nil || n != 2 {
			t.Errorf("NextNumber after a commit = %v, %v, want 2", n, err)
		}
		return nil
	})
}

// Two stores' staff send to one supplier at once: each takes a number and
// stores its order in one transaction. Postgres makes the second wait for
// the counter row, so the numbers are 1 and 2, and both orders commit.
func concurrentSendsGetTwoNumbers(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	items := []Bought{h.item(t, w), h.item(t, w)}
	var mu sync.Mutex
	var numbers []purchasing.Number
	var wg sync.WaitGroup
	for _, b := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := h.try(w.s.Scope(), func(ctx context.Context) error {
				n, err := h.Repo.NextNumber(ctx, w.s.Tenant)
				if err != nil {
					return err
				}
				so := h.newOrder(t, w.s, w.supplier, n, epoch, b)
				if err := h.Repo.InsertSupplierOrder(ctx, so, sentText, sentURL); err != nil {
					return err
				}
				mu.Lock()
				numbers = append(numbers, n)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("concurrent send: %v", err)
			}
		}()
	}
	wg.Wait()
	slices.Sort(numbers)
	if want := []purchasing.Number{1, 2}; !slices.Equal(numbers, want) {
		t.Errorf("numbers of two concurrent sends = %v, want %v", numbers, want)
	}
}
