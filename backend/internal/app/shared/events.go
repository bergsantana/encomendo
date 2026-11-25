package shared

import "context"

// Event is a domain event. Domain packages declare events as plain structs
// with exported fields, which the outbox stores as JSON.
type Event interface {
	// EventName is the stable name consumers select on, such as
	// "tenancy.member_invited". Renaming one breaks stored events.
	EventName() string
}

// EventPublisher writes events to the outbox (outbox_events) in the same
// transaction as the change that caused them, so an event exists if and only
// if its change committed. cmd/worker relays them later.
type EventPublisher interface {
	// Publish stores e in the transaction carried by ctx. It returns
	// ErrNoTransaction outside TxManager.WithinTx, and an error matching
	// shared.ErrInvalid when e has an empty name or cannot be encoded as
	// JSON.
	Publish(ctx context.Context, e Event) error
}
