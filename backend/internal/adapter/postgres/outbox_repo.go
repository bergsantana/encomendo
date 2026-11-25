package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// Outbox implements appshared.EventPublisher on outbox_events. An event is
// one more row in the caller's transaction, so it commits or rolls back
// with the change that caused it. Its tenant is the transaction's, never a
// field of the event.
type Outbox struct{}

// NewOutbox returns the outbox publisher.
func NewOutbox() *Outbox {
	return &Outbox{}
}

// Publish follows the appshared.EventPublisher contract.
func (*Outbox) Publish(ctx context.Context, e appshared.Event) error {
	c, ok := ctx.Value(txKey{}).(*txConn)
	if !ok {
		return appshared.ErrNoTransaction
	}
	if c.scope.TenantID.IsZero() {
		return appshared.ErrEventWithoutTenant
	}
	name, payload, err := encodeEvent(e)
	if err != nil {
		return err
	}
	err = c.db.WithContext(ctx).Exec(`INSERT INTO outbox_events (tenant_id, name, payload) VALUES (?, ?, CAST(? AS jsonb))`,
		c.scope.TenantID.String(), name, string(payload)).Error
	if err != nil {
		return fmt.Errorf("store event %s: %w", name, mapError(err))
	}
	return nil
}

// encodeEvent returns e's name and JSON, or appshared.ErrInvalidEvent for a
// nil or nameless event or one JSON cannot encode.
func encodeEvent(e appshared.Event) (string, []byte, error) {
	if e == nil {
		return "", nil, fmt.Errorf("publish a nil event: %w", appshared.ErrInvalidEvent)
	}
	if v := reflect.ValueOf(e); v.Kind() == reflect.Pointer && v.IsNil() {
		return "", nil, fmt.Errorf("publish a nil %T: %w", e, appshared.ErrInvalidEvent)
	}
	name := e.EventName()
	if name == "" {
		return "", nil, fmt.Errorf("publish %T without a name: %w", e, appshared.ErrInvalidEvent)
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return "", nil, fmt.Errorf("encode event %s: %w (%w)", name, appshared.ErrInvalidEvent, err)
	}
	return name, payload, nil
}
