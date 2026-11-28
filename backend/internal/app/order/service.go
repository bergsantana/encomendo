package order

import (
	"context"
	"fmt"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// idempotencyTTL is how long a key is remembered (ADR 0030).
const idempotencyTTL = 24 * time.Hour

// Deps are the Service's ports. Every one is required.
type Deps struct {
	Repo        Repository
	Catalog     Catalog
	Customers   Customers
	Workflows   Workflows
	Store       Store
	Tracking    TrackingURLs
	WhatsApp    WhatsApp
	Idempotency appshared.Idempotency
	Secrets     appshared.SecretGenerator
	Tx          appshared.TxManager
	Events      appshared.EventPublisher
	Clock       appshared.Clock
	IDs         appshared.IDGenerator
}

// Service implements UseCases.
type Service struct{ d Deps }

// NewService returns a Service over d, or an error naming the first
// dependency that is missing.
func NewService(d Deps) (*Service, error) {
	for _, dep := range []struct {
		name    string
		missing bool
	}{
		{"Repo", d.Repo == nil}, {"Catalog", d.Catalog == nil}, {"Customers", d.Customers == nil},
		{"Workflows", d.Workflows == nil}, {"Store", d.Store == nil}, {"Tracking", d.Tracking == nil},
		{"WhatsApp", d.WhatsApp == nil}, {"Idempotency", d.Idempotency == nil}, {"Secrets", d.Secrets == nil},
		{"Tx", d.Tx == nil}, {"Events", d.Events == nil}, {"Clock", d.Clock == nil}, {"IDs", d.IDs == nil},
	} {
		if dep.missing {
			return nil, fmt.Errorf("order service: %s is required", dep.name)
		}
	}
	return &Service{d: d}, nil
}

// now returns the time and the store's day: dates count in the store's
// time zone (ADR 0030).
func (s *Service) now(f StoreFacts) (time.Time, shared.CivilDate) {
	t := s.d.Clock.Now()
	return t, shared.DateOf(t, f.Location)
}

// load restores the order with the workflow it was made with.
func (s *Service) load(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (*order.Order, error) {
	r, err := s.d.Repo.ByID(ctx, tenant, id)
	if err != nil {
		return nil, fmt.Errorf("load order %s: %w", id, err)
	}
	wf, err := s.d.Workflows.Get(ctx, r.WorkflowID, r.WorkflowVersion)
	if err != nil {
		return nil, fmt.Errorf("workflow of order %s: %w", id, err)
	}
	o, err := order.Restore(r, wf)
	if err != nil {
		return nil, fmt.Errorf("restore order %s: %w", id, err)
	}
	return o, nil
}

// view builds the staff view of o for by, with its customer.
func (s *Service) view(ctx context.Context, o *order.Order, by shared.Principal, today shared.CivilDate) (View, error) {
	c, err := s.customerOf(ctx, o)
	if err != nil {
		return View{}, err
	}
	return s.viewWith(ctx, o, c, by, today)
}

// viewWith builds the staff view of o with its customer c, naming the staff
// who recorded and voided its payments.
func (s *Service) viewWith(ctx context.Context, o *order.Order, c CustomerFacts, by shared.Principal, today shared.CivilDate) (View, error) {
	staff := map[shared.UserID]string{}
	if ids := staffIDs(o); len(ids) > 0 {
		var err error
		if staff, err = s.d.Repo.StaffNames(ctx, ids); err != nil {
			return View{}, fmt.Errorf("staff names of order %s: %w", o.ID(), err)
		}
	}
	v := toView(o, c, staff, by, today)
	// The link has no text: it only opens the chat. The client never builds
	// a wa.me link (CLAUDE.md, rule 8).
	link, err := s.d.WhatsApp.Link(c.Phone, "")
	if err != nil {
		return View{}, fmt.Errorf("wa.me link of customer %s: %w", c.ID, err)
	}
	v.Customer.WhatsAppURL = link
	return v, nil
}

// customerOf reads the order's customer.
func (s *Service) customerOf(ctx context.Context, o *order.Order) (CustomerFacts, error) {
	c, err := s.d.Customers.Get(ctx, o.Customer())
	if err != nil {
		return CustomerFacts{}, fmt.Errorf("customer of order %s: %w", o.ID(), err)
	}
	return c, nil
}

// total is o's total. New, Restore's callers and Change keep every amount
// within the catalog's bound, so the sum cannot overflow; the zero amount
// stands in for that impossible case.
func total(o *order.Order) shared.Money {
	t, err := o.Total()
	if err != nil {
		return shared.Money{}
	}
	return t
}

// write is the shape of most order writes: authorize, then in one
// transaction load the order at the caller's version, let change act on it,
// save it with its events, and answer the staff view.
type write struct {
	perm    identity.Permission
	order   shared.OrderID
	version int
	// anyVersion skips the version check, for a write that changes nothing
	// the caller could have read stale (a new tracking link).
	anyVersion bool
	change     func(ctx context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error)
}

// writeCtx is what a write's change step gets.
type writeCtx struct {
	by    shared.Principal
	facts StoreFacts
	now   time.Time
	today shared.CivilDate
	// preview is set by Quote: the call computes what a write would do and
	// stores nothing, not even a customer's new address.
	preview bool
}

// writeCtx reads the store's facts and the clock once for a call, so every
// date it computes falls on the same store day.
func (s *Service) writeCtx(ctx context.Context, by shared.Principal) (writeCtx, error) {
	facts, err := s.d.Store.Facts(ctx)
	if err != nil {
		return writeCtx{}, fmt.Errorf("store facts: %w", err)
	}
	now, today := s.now(facts)
	return writeCtx{by: by, facts: facts, now: now, today: today}, nil
}

func (s *Service) write(ctx context.Context, w write) (View, error) {
	by, err := authz.Require(ctx, w.perm)
	if err != nil {
		return View{}, err
	}
	var out View
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		facts, err := s.d.Store.Facts(ctx)
		if err != nil {
			return fmt.Errorf("store facts: %w", err)
		}
		now, today := s.now(facts)
		o, err := s.load(ctx, by.TenantID(), w.order)
		if err != nil {
			return err
		}
		if !w.anyVersion {
			if err := o.CheckVersion(w.version); err != nil {
				return err
			}
		}
		events, err := w.change(ctx, o, writeCtx{by: by, facts: facts, now: now, today: today})
		if err != nil {
			return err
		}
		if err := s.save(ctx, o, events); err != nil {
			return err
		}
		out, err = s.savedView(ctx, o, by, today)
		return err
	})
	return out, err
}

// savedView is view after Update: the repository stored the next version,
// so the answer carries it for the caller's next write.
func (s *Service) savedView(ctx context.Context, o *order.Order, by shared.Principal, today shared.CivilDate) (View, error) {
	v, err := s.view(ctx, o, by, today)
	v.Version = o.Version() + 1
	return v, err
}

// save stores o and publishes its events in the caller's transaction.
func (s *Service) save(ctx context.Context, o *order.Order, events []appshared.Event) error {
	if err := s.d.Repo.Update(ctx, o); err != nil {
		return fmt.Errorf("save order %s: %w", o.ID(), err)
	}
	return s.publish(ctx, events)
}

func (s *Service) publish(ctx context.Context, events []appshared.Event) error {
	for _, e := range events {
		if err := s.d.Events.Publish(ctx, e); err != nil {
			return fmt.Errorf("publish %s: %w", e.EventName(), err)
		}
	}
	return nil
}

// requireManage returns shared.ErrForbidden unless by may manage orders.
func requireManage(by shared.Principal, what string) error {
	if identity.Allows(by.Role(), identity.PermOrderManage) {
		return nil
	}
	return fmt.Errorf("role %s may not %s: %w", by.Role(), what, shared.ErrForbidden)
}

// Get returns the order's staff view (order:read). Costs and margins are
// filled only for the owner.
func (s *Service) Get(ctx context.Context, id shared.OrderID) (View, error) {
	by, err := authz.Require(ctx, identity.PermOrderRead)
	if err != nil {
		return View{}, err
	}
	var out View
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		facts, err := s.d.Store.Facts(ctx)
		if err != nil {
			return fmt.Errorf("store facts: %w", err)
		}
		_, today := s.now(facts)
		o, err := s.load(ctx, by.TenantID(), id)
		if err != nil {
			return err
		}
		out, err = s.view(ctx, o, by, today)
		return err
	})
	return out, err
}
