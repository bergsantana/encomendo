package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Events is an EventPublisher that keeps committed events in memory. It
// works with the fake TxManager: an event published inside WithinTx is kept
// only if that transaction succeeds.
type Events struct {
	mu        sync.Mutex
	committed []appshared.Event
}

// NewEvents returns a publisher with no events.
func NewEvents() *Events {
	return &Events{}
}

// Publish follows the EventPublisher contract: it needs a transaction from
// the fake TxManager and refuses nameless events and events JSON cannot
// encode, as the outbox would.
func (e *Events) Publish(ctx context.Context, ev appshared.Event) error {
	tx := txFrom(ctx)
	if tx == nil {
		return appshared.ErrNoTransaction
	}
	if ev.EventName() == "" {
		return fmt.Errorf("publish event without a name: %w", shared.ErrInvalid)
	}
	if _, err := json.Marshal(ev); err != nil {
		return fmt.Errorf("encode event %s: %w (%w)", ev.EventName(), shared.ErrInvalid, err)
	}
	tx.stage(func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.committed = append(e.committed, ev)
	})
	return nil
}

// Published returns the committed events, oldest first.
func (e *Events) Published() []appshared.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.committed)
}

// Names returns the names of the committed events, oldest first.
func (e *Events) Names() []string {
	events := e.Published()
	names := make([]string, len(events))
	for i, ev := range events {
		names[i] = ev.EventName()
	}
	return names
}
