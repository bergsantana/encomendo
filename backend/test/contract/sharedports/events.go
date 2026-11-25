package sharedports

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// EventsHarness is an EventPublisher with the TxManager it writes through,
// and a way to read back what was committed.
type EventsHarness struct {
	Tx        appshared.TxManager
	Publisher appshared.EventPublisher
	// Committed returns the names of the stored events, oldest first.
	Committed func(t *testing.T) []string
}

type namedEvent struct {
	Name string `json:"-"`
	Note string `json:"note"`
}

func (e namedEvent) EventName() string { return e.Name }

type unencodableEvent struct {
	Callback func() `json:"callback"`
}

func (unencodableEvent) EventName() string { return "contract.unencodable" }

// RunEventPublisherContract checks the outbox promise: an event is stored
// if and only if its transaction commits, Publish needs a transaction, and
// events without a name or that JSON cannot encode are refused.
func RunEventPublisherContract(t *testing.T, newHarness func(t *testing.T) EventsHarness) {
	t.Run("stored when the transaction commits", func(t *testing.T) { eventsCommit(t, newHarness(t)) })
	t.Run("dropped when the transaction rolls back", func(t *testing.T) { eventsRollback(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { eventsNeedTx(t, newHarness(t)) })
	t.Run("refuses invalid events", func(t *testing.T) { eventsInvalid(t, newHarness(t)) })
}

func eventsCommit(t *testing.T, h EventsHarness) {
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := h.Publisher.Publish(ctx, namedEvent{Name: "contract.first"}); err != nil {
			return err
		}
		return h.Publisher.Publish(ctx, namedEvent{Name: "contract.second"})
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	if got, want := h.Committed(t), []string{"contract.first", "contract.second"}; !slices.Equal(got, want) {
		t.Errorf("committed = %v, want %v", got, want)
	}
}

func eventsRollback(t *testing.T, h EventsHarness) {
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := h.Publisher.Publish(ctx, namedEvent{Name: "contract.rolled_back"}); err != nil {
			return err
		}
		return errFromFn
	})
	if !errors.Is(err, errFromFn) {
		t.Fatalf("WithinTx error = %v, want fn's error", err)
	}
	if got := h.Committed(t); len(got) != 0 {
		t.Errorf("committed = %v after a rollback, want none", got)
	}
}

func eventsNeedTx(t *testing.T, h EventsHarness) {
	err := h.Publisher.Publish(context.Background(), namedEvent{Name: "contract.no_tx"})
	if !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Publish outside a transaction error = %v, want ErrNoTransaction", err)
	}
}

func eventsInvalid(t *testing.T, h EventsHarness) {
	var nameless, unencodable error
	err := h.Tx.WithinTx(context.Background(), func(ctx context.Context) error {
		nameless = h.Publisher.Publish(ctx, namedEvent{})
		unencodable = h.Publisher.Publish(ctx, unencodableEvent{Callback: func() {}})
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	if !errors.Is(nameless, shared.ErrInvalid) {
		t.Errorf("Publish without a name error = %v, want shared.ErrInvalid", nameless)
	}
	if !errors.Is(unencodable, shared.ErrInvalid) {
		t.Errorf("Publish of an unencodable event error = %v, want shared.ErrInvalid", unencodable)
	}
	if got := h.Committed(t); len(got) != 0 {
		t.Errorf("committed = %v, want the refused events left out", got)
	}
}
