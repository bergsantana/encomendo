package order

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CreatedView is a new order, with its tracking link this once (the server
// keeps only its hash) and, when it was sent to the customer, the wa.me
// link of the message. A replay carries neither link.
type CreatedView struct {
	Order       View
	TrackingURL string
	WhatsAppURL string
}

// msgCustomer is the design document's refusal (Novo pedido, Cliente).
const msgCustomer = "Informe o cliente do pedido."

// Create makes an order ("Novo pedido", order:write; a discount, deposit or
// days other than the defaults need order:manage). The order takes the
// store's current status workflow and keeps that version for life (ADR
// 0004). With SendToCustomer it moves to the status after the first and
// answers the wa.me message with the tracking link. The Idempotency-Key is
// checked after the permission, so a replay never answers a role that may
// not create; a replay answers the order as it is now.
//
// It returns ErrIdempotencyKeyRequired, appshared.ErrIdempotencyKeyReused,
// shared.ErrForbidden, a *shared.ValidationError, and the catalog's
// refusals (archived items, swap rules).
func (s *Service) Create(ctx context.Context, in CreateInput) (CreatedView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return CreatedView{}, err
	}
	if in.Idempotency.IsZero() {
		return CreatedView{}, ErrIdempotencyKeyRequired
	}
	var out CreatedView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		facts, err := s.d.Store.Facts(ctx)
		if err != nil {
			return fmt.Errorf("store facts: %w", err)
		}
		if replayed, ok, err := s.replayCreate(ctx, in.Idempotency, by, facts); err != nil || ok {
			out = replayed
			return err
		}
		out, err = s.create(ctx, in, by, facts)
		return err
	})
	return out, err
}

func (s *Service) replayCreate(ctx context.Context, k appshared.IdempotencyKey, by shared.Principal, f StoreFacts) (CreatedView, bool, error) {
	resource, ok, err := s.d.Idempotency.Recall(ctx, k)
	if err != nil || !ok {
		return CreatedView{}, false, err
	}
	id, err := shared.ParseOrderID(resource)
	if err != nil {
		return CreatedView{}, false, fmt.Errorf("remembered order %q: %w", resource, err)
	}
	o, err := s.load(ctx, by.TenantID(), id)
	if err != nil {
		return CreatedView{}, false, err
	}
	_, today := s.now(f)
	v, err := s.view(ctx, o, by, today)
	return CreatedView{Order: v}, true, err
}

func (s *Service) create(ctx context.Context, in CreateInput, by shared.Principal, f StoreFacts) (CreatedView, error) {
	now, today := s.now(f)
	c, err := s.customerFor(ctx, in)
	if err != nil {
		return CreatedView{}, err
	}
	o, token, events, err := s.build(ctx, in, c, by, f)
	if err != nil {
		return CreatedView{}, err
	}
	if err := s.d.Repo.Insert(ctx, o); err != nil {
		return CreatedView{}, fmt.Errorf("insert order: %w", err)
	}
	if err := s.d.Idempotency.Remember(ctx, in.Idempotency, o.ID().String(), idempotencyTTL); err != nil {
		return CreatedView{}, fmt.Errorf("remember idempotency key: %w", err)
	}
	if err := s.publish(ctx, events); err != nil {
		return CreatedView{}, err
	}
	link, err := s.d.Tracking.URL(f.Slug, token)
	if err != nil {
		return CreatedView{}, fmt.Errorf("tracking url: %w", err)
	}
	out := CreatedView{Order: toView(o, c, by, today), TrackingURL: link}
	if in.SendToCustomer {
		text := order.CreatedMessage(messageFacts(f, c, o), link)
		out.WhatsAppURL, err = s.message(ctx, outgoing{order: o, to: c, kind: MessageCreated, text: text, link: link, by: by.UserID(), at: now})
	}
	return out, err
}

// build makes the order in memory: its lines from the catalog, its terms,
// the store's next number, a tracking token and the current workflow.
func (s *Service) build(ctx context.Context, in CreateInput, c CustomerFacts, by shared.Principal, f StoreFacts) (*order.Order, shared.Secret, []appshared.Event, error) {
	now, today := s.now(f)
	wf, err := s.d.Workflows.Current(ctx)
	if err != nil {
		return nil, shared.Secret{}, nil, fmt.Errorf("current workflow: %w", err)
	}
	lines, err := s.newLines(ctx, in.Lines)
	if err != nil {
		return nil, shared.Secret{}, nil, err
	}
	terms, err := s.newTerms(by, f, lines, in.Terms, c)
	if err != nil {
		return nil, shared.Secret{}, nil, err
	}
	number, err := s.d.Repo.NextNumber(ctx, by.TenantID())
	if err != nil {
		return nil, shared.Secret{}, nil, fmt.Errorf("next order number: %w", err)
	}
	token, err := s.d.Secrets.NewSecret(ctx)
	if err != nil {
		return nil, shared.Secret{}, nil, fmt.Errorf("tracking token: %w", err)
	}
	o, err := order.New(order.NewSpec{
		Tenant: by.TenantID(), ID: shared.OrderID(s.d.IDs.NewID()), Number: number, Customer: c.ID,
		Workflow: wf, Lines: lines, Terms: terms, Tracking: token.Hash(), By: by, Now: now, Today: today,
	})
	if err != nil {
		return nil, shared.Secret{}, nil, err
	}
	events := []appshared.Event{order.Created{Order: o.ID(), Number: o.Number(), Workflow: wf.ID(), Version: wf.Version()}}
	if in.SendToCustomer {
		next := wf.Statuses()[1] // every workflow's second status is awaiting_deposit: no guard applies
		if err := o.Advance(next.Key, nil, by, today, now); err != nil {
			return nil, shared.Secret{}, nil, err
		}
		events = append(events, order.Advanced{Order: o.ID(), From: wf.First().Key, To: next.Key, Phase: next.Phase})
	}
	return o, token, events, nil
}

func (s *Service) newTerms(by shared.Principal, f StoreFacts, lines []order.LineSpec, in TermsInput, c CustomerFacts) (order.Terms, error) {
	base, err := newBaseline(f, lines)
	if err != nil {
		return order.Terms{}, err
	}
	return resolveTerms(by, base, in, c)
}

// customerFor finds the order's customer: by id, or by the new customer's
// WhatsApp number, made when the store has none with it.
func (s *Service) customerFor(ctx context.Context, in CreateInput) (CustomerFacts, error) {
	switch {
	case !in.Customer.IsZero() && in.New == nil:
		return s.d.Customers.Get(ctx, in.Customer)
	case in.Customer.IsZero() && in.New != nil:
		return s.d.Customers.FindOrCreate(ctx, in.New.Name, in.New.Phone)
	}
	return CustomerFacts{}, shared.NewValidationError(shared.FieldError{Field: "customerId", Message: msgCustomer})
}
