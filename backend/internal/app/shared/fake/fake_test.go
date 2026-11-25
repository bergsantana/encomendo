package fake

import (
	"context"
	"errors"
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
