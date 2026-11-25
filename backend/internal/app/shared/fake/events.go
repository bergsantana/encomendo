package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Events is an EventPublisher that keeps committed events in memory. It
// works with the fake TxManager: an event published inside WithinTx is kept
// only if that transaction commits.
type Events struct {
	mu        sync.Mutex
	committed []Record
}

// Record is one committed event as the outbox would hold it.
type Record struct {
	Name     string
	TenantID shared.TenantID
	// Payload is the JSON the outbox stores.
	Payload []byte
	// Event is Payload decoded back into the event's type, so a test sees
	// what a consumer will read, not the Go value that was published.
	Event appshared.Event
}

// NewEvents returns a publisher with no events.
func NewEvents() *Events {
	return &Events{}
}

// Publish follows the EventPublisher contract: it needs a transaction from
// the fake TxManager with a tenant in its scope, and refuses nil, nameless
// and unencodable events. It also refuses an event whose JSON does not
// decode back into its own type, which a consumer could not read.
func (e *Events) Publish(ctx context.Context, ev appshared.Event) error {
	tx := txFrom(ctx)
	if tx == nil {
		return appshared.ErrNoTransaction
	}
	if tx.scope.TenantID.IsZero() {
		return appshared.ErrEventWithoutTenant
	}
	rec, err := record(ev)
	if err != nil {
		return err
	}
	rec.TenantID = tx.scope.TenantID
	tx.stage(func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.committed = append(e.committed, rec)
	})
	return nil
}

func record(ev appshared.Event) (Record, error) {
	if ev == nil {
		return Record{}, fmt.Errorf("publish a nil event: %w", appshared.ErrInvalidEvent)
	}
	if v := reflect.ValueOf(ev); v.Kind() == reflect.Pointer && v.IsNil() {
		return Record{}, fmt.Errorf("publish a nil %T: %w", ev, appshared.ErrInvalidEvent)
	}
	name := ev.EventName()
	if name == "" {
		return Record{}, fmt.Errorf("publish %T without a name: %w", ev, appshared.ErrInvalidEvent)
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return Record{}, fmt.Errorf("encode event %s: %w (%w)", name, appshared.ErrInvalidEvent, err)
	}
	decoded := reflect.New(reflect.TypeOf(ev))
	if err := json.Unmarshal(payload, decoded.Interface()); err != nil {
		return Record{}, fmt.Errorf("event %s does not decode back: %w (%w)", name, appshared.ErrInvalidEvent, err)
	}
	back, _ := decoded.Elem().Interface().(appshared.Event) // same type as ev, so it is an Event
	return Record{Name: name, Payload: payload, Event: back}, nil
}

// Records returns the committed events, oldest first.
func (e *Events) Records() []Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.committed)
}

// Published returns the committed events as a consumer would decode them,
// oldest first.
func (e *Events) Published() []appshared.Event {
	records := e.Records()
	events := make([]appshared.Event, len(records))
	for i, r := range records {
		events[i] = r.Event
	}
	return events
}

// Names returns the names of the committed events, oldest first.
func (e *Events) Names() []string {
	records := e.Records()
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Name
	}
	return names
}
