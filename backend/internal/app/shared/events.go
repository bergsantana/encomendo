package shared

import (
	"context"
	"errors"
)

// Event publishing misuse errors. They match no domain sentinel, so they
// reach the API as a logged 500 (ADR 0009).
var (
	// ErrInvalidEvent means the event is nil, has an empty name, or cannot
	// be encoded as JSON.
	ErrInvalidEvent = errors.New("invalid event")
	// ErrEventWithoutTenant means Publish ran in a scope with no tenant. The
	// outbox is a tenant table under RLS, so every event belongs to a store;
	// work done before a store exists (sign-up, password reset) publishes
	// nothing.
	ErrEventWithoutTenant = errors.New("event published without a tenant")
)

// Event is a domain event. Domain packages declare events as plain structs
// with exported fields, which the outbox stores as JSON; ids and money
// marshal as strings and {"cents","currency"} objects.
type Event interface {
	// EventName is the stable name consumers select on, such as
	// "tenancy.member_invited". Renaming one breaks stored events.
	EventName() string
}

// EventPublisher writes events to the outbox (outbox_events) in the same
// transaction as the change that caused them, so an event exists if and only
// if its change committed. cmd/worker relays them later.
type EventPublisher interface {
	// Publish stores e in the transaction carried by ctx, under the tenant
	// of that transaction's scope. It returns ErrNoTransaction outside
	// TxManager.WithinTx, ErrEventWithoutTenant when the scope has no
	// tenant, and ErrInvalidEvent for a nil or nameless event or one that
	// JSON cannot encode. An event published in a nested WithinTx that
	// fails is dropped with the rest of that nested work.
	Publish(ctx context.Context, e Event) error
}
