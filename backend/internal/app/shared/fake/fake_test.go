package fake

import (
	"context"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The contract suites in test/contract/sharedports cover the port
// behaviour; these tests cover the helpers only use case tests call.

func TestClock_AdvanceAndSet(t *testing.T) {
	t.Parallel()

	manaus := time.FixedZone("AMT", -4*60*60)
	c := NewClock(time.Date(2026, 11, 24, 8, 0, 0, 0, manaus))
	if got := c.Now(); got.Location() != time.UTC || got.Hour() != 12 {
		t.Errorf("NewClock kept %v, want 12:00 UTC", got)
	}
	c.Advance(90 * time.Minute)
	if got := c.Now(); got.Hour() != 13 || got.Minute() != 30 {
		t.Errorf("after Advance = %v", got)
	}
	c.Set(time.Date(2027, 1, 1, 0, 0, 0, 0, manaus))
	if got := c.Now(); got.Location() != time.UTC || got.Hour() != 4 {
		t.Errorf("after Set = %v, want 04:00 UTC", got)
	}
}

func TestTxManager_RecordsEveryScope(t *testing.T) {
	t.Parallel()

	tx := NewTxManager()
	outer := appshared.Scope{TenantID: shared.TenantID(shared.NewV7(1, [10]byte{1}))}
	err := tx.WithinTx(appshared.WithScope(context.Background(), outer), func(ctx context.Context) error {
		return tx.WithinTx(ctx, func(context.Context) error { return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := tx.Scopes(); !slices.Equal(got, []appshared.Scope{outer, outer}) {
		t.Errorf("Scopes() = %+v, want the outer scope twice", got)
	}
}

func TestIDs_SameClockSameSequence(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	a, b := NewIDs(NewClock(start)), NewIDs(NewClock(start))
	for range 3 {
		if x, y := a.NewID(), b.NewID(); x != y {
			t.Fatalf("generators diverged: %s vs %s", x, y)
		}
	}
}
