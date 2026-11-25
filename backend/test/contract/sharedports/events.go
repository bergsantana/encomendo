package sharedports

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// StoredEvent is one committed outbox row as the suite reads it back.
type StoredEvent struct {
	Name    string
	Payload json.RawMessage
}

// EventsHarness is an EventPublisher with the TxManager it writes through,
// and a way to read back what was committed.
type EventsHarness struct {
	Tx        appshared.TxManager
	Publisher appshared.EventPublisher
	// Committed returns the stored events of TenantID, oldest first.
	Committed func(t *testing.T) []StoredEvent
	// TenantID is a tenant the publisher may write events for. For Postgres
	// it must exist.
	TenantID shared.TenantID
}

type namedEvent struct {
	Name   string          `json:"-"`
	Note   string          `json:"note"`
	Tenant shared.TenantID `json:"tenant"`
	Price  shared.Money    `json:"price"`
}

func (e namedEvent) EventName() string { return e.Name }

type unencodableEvent struct {
	Callback func() `json:"callback"`
}

func (unencodableEvent) EventName() string { return "contract.unencodable" }

// RunEventPublisherContract checks the outbox promise: an event is stored,
// as JSON, if and only if its transaction commits (nested savepoints
// included); Publish needs a transaction and a tenant; nil, nameless and
// unencodable events are refused.
func RunEventPublisherContract(t *testing.T, newHarness func(t *testing.T) EventsHarness) {
	t.Run("stored when the transaction commits", func(t *testing.T) { eventsCommit(t, newHarness(t)) })
	t.Run("dropped when the transaction rolls back", func(t *testing.T) { eventsRollback(t, newHarness(t)) })
	t.Run("nested events join the outer transaction", func(t *testing.T) { eventsNestedJoin(t, newHarness(t)) })
	t.Run("a failed nested call drops only its events", func(t *testing.T) { eventsSavepoint(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { eventsNeedTx(t, newHarness(t)) })
	t.Run("needs a tenant", func(t *testing.T) { eventsNeedTenant(t, newHarness(t)) })
	t.Run("refuses invalid events", func(t *testing.T) { eventsInvalid(t, newHarness(t)) })
}

func tenantCtx(h EventsHarness) context.Context {
	return appshared.WithScope(context.Background(), appshared.Scope{TenantID: h.TenantID})
}

func committedNames(t *testing.T, h EventsHarness) []string {
	t.Helper()
	var names []string
	for _, e := range h.Committed(t) {
		names = append(names, e.Name)
	}
	return names
}

func eventsCommit(t *testing.T, h EventsHarness) {
	price, err := shared.NewMoney(45000, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	first := namedEvent{Name: "contract.first", Note: "Banco de couro", Tenant: h.TenantID, Price: price}
	err = h.Tx.WithinTx(tenantCtx(h), func(ctx context.Context) error {
		if err := h.Publisher.Publish(ctx, first); err != nil {
			return err
		}
		return h.Publisher.Publish(ctx, namedEvent{Name: "contract.second"})
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	stored := h.Committed(t)
	if got, want := committedNames(t, h), []string{"contract.first", "contract.second"}; !slices.Equal(got, want) {
		t.Fatalf("committed = %v, want %v", got, want)
	}
	// The payload is what consumers will read: ids as strings, money as
	// {"cents","currency"} (ADR 0009).
	var back namedEvent
	if err := json.Unmarshal(stored[0].Payload, &back); err != nil {
		t.Fatalf("payload %s does not decode: %v", stored[0].Payload, err)
	}
	back.Name = first.Name
	if back != first {
		t.Errorf("payload decodes to %+v, want %+v", back, first)
	}
}

func eventsRollback(t *testing.T, h EventsHarness) {
	err := h.Tx.WithinTx(tenantCtx(h), func(ctx context.Context) error {
		if err := h.Publisher.Publish(ctx, namedEvent{Name: "contract.rolled_back"}); err != nil {
			return err
		}
		return errFromFn
	})
	if !errors.Is(err, errFromFn) {
		t.Fatalf("WithinTx error = %v, want fn's error", err)
	}
	if got := committedNames(t, h); len(got) != 0 {
		t.Errorf("committed = %v after a rollback, want none", got)
	}
}

func eventsNestedJoin(t *testing.T, h EventsHarness) {
	// The nested call succeeds but the outer one fails: if the nested call
	// had its own transaction, its event would survive.
	err := h.Tx.WithinTx(tenantCtx(h), func(ctx context.Context) error {
		if err := h.Tx.WithinTx(ctx, func(ctx context.Context) error {
			return h.Publisher.Publish(ctx, namedEvent{Name: "contract.nested"})
		}); err != nil {
			return err
		}
		return errFromFn
	})
	if !errors.Is(err, errFromFn) {
		t.Fatalf("WithinTx error = %v, want fn's error", err)
	}
	if got := committedNames(t, h); len(got) != 0 {
		t.Errorf("committed = %v, want the nested event rolled back with the outer transaction", got)
	}
}

func eventsSavepoint(t *testing.T, h EventsHarness) {
	err := h.Tx.WithinTx(tenantCtx(h), func(ctx context.Context) error {
		if err := h.Publisher.Publish(ctx, namedEvent{Name: "contract.outer"}); err != nil {
			return err
		}
		inner := h.Tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := h.Publisher.Publish(ctx, namedEvent{Name: "contract.inner"}); err != nil {
				return err
			}
			return errFromFn
		})
		if !errors.Is(inner, errFromFn) {
			return inner
		}
		return nil // the outer work goes on without the failed inner part
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	if got, want := committedNames(t, h), []string{"contract.outer"}; !slices.Equal(got, want) {
		t.Errorf("committed = %v, want %v", got, want)
	}
}

func eventsNeedTx(t *testing.T, h EventsHarness) {
	err := h.Publisher.Publish(tenantCtx(h), namedEvent{Name: "contract.no_tx"})
	if !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("Publish outside a transaction error = %v, want ErrNoTransaction", err)
	}
}

func eventsNeedTenant(t *testing.T, h EventsHarness) {
	var publishErr error
	userOnly := appshared.WithScope(context.Background(), appshared.Scope{UserID: txUser})
	err := h.Tx.WithinTx(userOnly, func(ctx context.Context) error {
		publishErr = h.Publisher.Publish(ctx, namedEvent{Name: "contract.no_tenant"})
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	if !errors.Is(publishErr, appshared.ErrEventWithoutTenant) {
		t.Errorf("Publish without a tenant error = %v, want ErrEventWithoutTenant", publishErr)
	}
}

func eventsInvalid(t *testing.T, h EventsHarness) {
	var errs []error
	err := h.Tx.WithinTx(tenantCtx(h), func(ctx context.Context) error {
		errs = append(errs,
			h.Publisher.Publish(ctx, namedEvent{}),
			h.Publisher.Publish(ctx, unencodableEvent{Callback: func() {}}),
			h.Publisher.Publish(ctx, nil),
		)
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTx error = %v", err)
	}
	for i, name := range []string{"nameless", "unencodable", "nil"} {
		if !errors.Is(errs[i], appshared.ErrInvalidEvent) {
			t.Errorf("Publish of a %s event error = %v, want ErrInvalidEvent", name, errs[i])
		}
	}
	if got := committedNames(t, h); len(got) != 0 {
		t.Errorf("committed = %v, want the refused events left out", got)
	}
}
