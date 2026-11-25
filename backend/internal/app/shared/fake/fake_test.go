package fake

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
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

func TestIDs_KeepOrderWhenTheClockMovesBack(t *testing.T) {
	t.Parallel()

	clock := NewClock(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	ids := NewIDs(clock)
	first := ids.NewID()
	clock.Set(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if second := ids.NewID(); second.String() <= first.String() {
		t.Errorf("%s issued after %s sorts before it", second, first)
	}
}

type pointerEvent struct{ Note string }

func (*pointerEvent) EventName() string { return "fake.pointer" }

// halfEvent encodes, but its JSON does not decode back into its own type.
type halfEvent struct{}

func (halfEvent) EventName() string            { return "fake.half" }
func (halfEvent) MarshalJSON() ([]byte, error) { return []byte(`"not an object"`), nil }
func (*halfEvent) UnmarshalJSON([]byte) error  { return errors.New("never decodes") }

func TestEvents_RecordTheTenantAndADecodedCopy(t *testing.T) {
	t.Parallel()

	tenant := shared.TenantID(shared.NewV7(1, [10]byte{1}))
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
	tx, events := NewTxManager(), NewEvents()
	published := &pointerEvent{Note: "x"}
	err := tx.WithinTx(ctx, func(ctx context.Context) error { return events.Publish(ctx, published) })
	if err != nil {
		t.Fatal(err)
	}
	recs := events.Records()
	if len(recs) != 1 || recs[0].TenantID != tenant || string(recs[0].Payload) != `{"Note":"x"}` {
		t.Fatalf("records = %+v", recs)
	}
	if got := events.Names(); !slices.Equal(got, []string{"fake.pointer"}) {
		t.Errorf("Names() = %v", got)
	}
	got, ok := events.Published()[0].(*pointerEvent)
	if !ok || got == published || got.Note != "x" {
		t.Errorf("Published()[0] = %#v, want a decoded copy, not the published pointer", events.Published()[0])
	}
}

func TestEvents_RefuseTypedNilAndUndecodableEvents(t *testing.T) {
	t.Parallel()

	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: shared.TenantID(shared.NewV7(1, [10]byte{1}))})
	tx, events := NewTxManager(), NewEvents()
	var typedNil *pointerEvent
	var errs []error
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		errs = append(errs, events.Publish(ctx, typedNil), events.Publish(ctx, halfEvent{}))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range errs {
		if !errors.Is(e, appshared.ErrInvalidEvent) {
			t.Errorf("case %d: error = %v, want ErrInvalidEvent", i, e)
		}
	}
}

func TestInTx_SeesOnlyTheFakeTransaction(t *testing.T) {
	t.Parallel()

	if InTx(context.Background()) {
		t.Error("InTx is true outside a transaction")
	}
	var inside bool
	err := NewTxManager().WithinTx(context.Background(), func(ctx context.Context) error {
		inside = InTx(ctx)
		return nil
	})
	if err != nil || !inside {
		t.Errorf("InTx inside WithinTx = %v (err %v), want true", inside, err)
	}
}

func TestOnRollback_UndoesOnlyWhatFailed(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	rows := map[string]int{"kept": 1}
	write := func(ctx context.Context, k string, v int) {
		mu.Lock()
		defer mu.Unlock()
		KeepOnRollback(ctx, &mu, rows, k)
		rows[k] = v
	}
	boom := errors.New("boom")
	tx := NewTxManager()

	err := tx.WithinTx(context.Background(), func(ctx context.Context) error {
		write(ctx, "kept", 2)
		write(ctx, "committed", 3)
		nested := tx.WithinTx(ctx, func(ctx context.Context) error {
			write(ctx, "savepoint", 4)
			write(ctx, "kept", 5)
			return boom
		})
		if !errors.Is(nested, boom) {
			t.Errorf("nested = %v", nested)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"kept": 2, "committed": 3}; !maps.Equal(rows, want) {
		t.Errorf("after a failed savepoint: %v, want %v", rows, want)
	}

	err = tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := tx.WithinTx(ctx, func(ctx context.Context) error { write(ctx, "released", 6); return nil }); err != nil {
			return err
		}
		write(ctx, "kept", 7)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if want := map[string]int{"kept": 2, "committed": 3}; !maps.Equal(rows, want) {
		t.Errorf("after a failed transaction: %v, want %v; a released savepoint's writes go with it", rows, want)
	}

	OnRollback(context.Background(), func() { t.Error("an undo outside a transaction ran") })
}

func TestSecrets_AreDistinctAndRepeatable(t *testing.T) {
	t.Parallel()

	a, b := NewSecrets(), NewSecrets()
	first, _ := a.NewSecret(context.Background())
	second, _ := a.NewSecret(context.Background())
	if first == second {
		t.Error("two secrets in a row are equal")
	}
	if again, _ := b.NewSecret(context.Background()); again != first {
		t.Error("a second generator does not give the same sequence")
	}
}

func TestSecrets_Fail(t *testing.T) {
	t.Parallel()

	boom := errors.New("no entropy")
	s := NewSecrets()
	s.Fail(boom)
	if _, err := s.NewSecret(context.Background()); !errors.Is(err, boom) {
		t.Errorf("NewSecret after Fail = %v, want %v", err, boom)
	}
	s.Fail(nil)
	if _, err := s.NewSecret(context.Background()); err != nil {
		t.Errorf("NewSecret after Fail(nil) = %v", err)
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
