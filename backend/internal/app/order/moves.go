package order

import (
	"context"
	"fmt"
	"slices"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RecordPayment records a payment received by hand (order:write, so staff
// can take the Saldo at pickup; ADR 0030). The payment that makes the
// deposit paid releases the order and publishes order.deposit_confirmed.
// The Idempotency-Key is checked after the permission; a replay answers the
// order as it is now. It returns ErrIdempotencyKeyRequired,
// appshared.ErrIdempotencyKeyReused and the domain's refusals
// (order.ErrPaymentExceedsDue, order.ErrOrderClosed).
func (s *Service) RecordPayment(ctx context.Context, in PaymentInput) (View, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return View{}, err
	}
	if in.Idempotency.IsZero() {
		return View{}, ErrIdempotencyKeyRequired
	}
	var out View
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		o, err := s.load(ctx, by.TenantID(), in.Order)
		if err != nil {
			return err
		}
		if replayed, ok, err := s.replayPayment(ctx, o, in, w); err != nil || ok {
			out = replayed
			return err
		}
		if err := o.CheckVersion(in.Version); err != nil {
			return err
		}
		id, events, err := s.pay(o, in, w)
		if err != nil {
			return err
		}
		if err := s.save(ctx, o, events); err != nil {
			return err
		}
		if err := s.d.Idempotency.Remember(ctx, in.Idempotency, id.String(), idempotencyTTL); err != nil {
			return fmt.Errorf("remember idempotency key: %w", err)
		}
		out, err = s.savedView(ctx, o, by, w.today)
		return err
	})
	return out, err
}

// replayPayment answers a key already used: the order, as it is now, when
// the key recorded one of its payments. A key that made anything else
// (another order's payment, an order) was reused for another request
// (security review of 4.10).
func (s *Service) replayPayment(ctx context.Context, o *order.Order, in PaymentInput, w writeCtx) (View, bool, error) {
	resource, ok, err := s.d.Idempotency.Recall(ctx, in.Idempotency)
	if err != nil || !ok {
		return View{}, false, err
	}
	id, err := shared.ParsePaymentID(resource)
	if err != nil || !slices.ContainsFunc(o.Payments(), func(p order.Payment) bool { return p.ID() == id }) {
		return View{}, false, appshared.ErrIdempotencyKeyReused
	}
	v, err := s.view(ctx, o, w.by, w.today)
	return v, true, err
}

func (s *Service) pay(o *order.Order, in PaymentInput, w writeCtx) (shared.PaymentID, []appshared.Event, error) {
	by, now, today := w.by, w.now, w.today
	amount, err := shared.NewMoney(in.AmountCents, o.Terms().Freight.Currency())
	if err != nil {
		return shared.PaymentID{}, nil, err
	}
	id := shared.PaymentID(s.d.IDs.NewID())
	confirmed, err := o.RecordPayment(order.PaymentSpec{
		ID: id, Kind: in.Kind, Amount: amount, Method: in.Method, By: by, At: now, Today: today,
	})
	if err != nil {
		return shared.PaymentID{}, nil, err
	}
	events := []appshared.Event{order.PaymentRecorded{Order: o.ID(), Payment: id, Kind: in.Kind}}
	if confirmed {
		release, _ := o.ReleaseDate() // a confirmed deposit releases the order
		events = append(events, order.DepositConfirmed{Order: o.ID(), ReleaseDate: release})
	}
	return id, events, nil
}

// VoidPayment voids a wrong payment (order:override, the owner's; ADR 0030).
func (s *Service) VoidPayment(ctx context.Context, in VoidPaymentInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderOverride, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			if err := o.VoidPayment(in.Payment, in.Reason, w.by, w.now); err != nil {
				return nil, err
			}
			return []appshared.Event{order.PaymentVoided{Order: o.ID(), Payment: in.Payment}}, nil
		}})
}

// Release lets the order go ahead without the deposit (order:override, the
// owner's; docs/architecture.md section 4.2).
func (s *Service) Release(ctx context.Context, in VersionInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderOverride, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			if err := o.Release(w.by, w.today, w.now); err != nil {
				return nil, err
			}
			release, _ := o.ReleaseDate() // Release just set it
			return []appshared.Event{order.Released{Order: o.ID(), ReleaseDate: release}}, nil
		}})
}

// Advance moves the order to another status of its workflow (order:write).
// The domain checks the guard of every phase it enters.
func (s *Service) Advance(ctx context.Context, in AdvanceInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderWrite, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			from := o.Status().Key
			if err := o.Advance(in.To, in.Handover, w.by, w.today, w.now); err != nil {
				return nil, err
			}
			return []appshared.Event{order.Advanced{Order: o.ID(), From: from, To: o.Status().Key, Phase: o.Phase()}}, nil
		}})
}

// MoveItem moves one item along its sourcing mode's lifecycle
// (order:write; ADR 0030: by hand until milestones 5 and 6).
func (s *Service) MoveItem(ctx context.Context, in MoveItemInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderWrite, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			if err := o.MoveItem(in.Item, in.To, in.Check, w.today, w.now, w.by); err != nil {
				return nil, err
			}
			for _, i := range o.Items() {
				if i.ID() == in.Item {
					return []appshared.Event{order.ItemMoved{Order: o.ID(), Item: in.Item, To: i.Status()}}, nil
				}
			}
			return nil, nil
		}})
}

// UseBackupSupplier switches a bought item still to be bought to its backup
// supplier ("Usar o fornecedor reserva", order:manage).
func (s *Service) UseBackupSupplier(ctx context.Context, in ItemVersionInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderManage, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			return nil, o.UseBackupSupplier(in.Item, w.today, w.now, w.by)
		}})
}

// Cancel cancels the order (order:manage). Payments stay as recorded.
// Milestone 5 cancels its unsent purchases and flags the sent ones, and
// milestone 6 releases its stock, each from order.cancelled.
func (s *Service) Cancel(ctx context.Context, in CancelInput) (View, error) {
	return s.write(ctx, write{perm: identity.PermOrderManage, order: in.Order, version: in.Version,
		change: func(_ context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			if err := o.Cancel(in.Reason, w.by, w.now); err != nil {
				return nil, err
			}
			return []appshared.Event{order.Cancelled{Order: o.ID()}}, nil
		}})
}

// ChangeDeliveryDate moves the date promised to the client (order:manage)
// and answers the wa.me message that warns them. Nothing is sent
// automatically (docs/architecture.md section 4.2).
func (s *Service) ChangeDeliveryDate(ctx context.Context, in DateInput) (MessagedView, error) {
	var wa string
	v, err := s.write(ctx, write{perm: identity.PermOrderManage, order: in.Order, version: in.Version,
		change: func(ctx context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			old, err := o.ChangeDeliveryDate(in.Date, in.Reason, w.by, w.today, w.now)
			if err != nil {
				return nil, err
			}
			c, err := s.customerOf(ctx, o)
			if err != nil {
				return nil, err
			}
			mf := messageFacts(w.facts, c, o)
			mf.OldDate, mf.Reason = old, in.Reason
			wa, err = s.message(ctx, outgoing{order: o, to: c, kind: MessageDateChanged, text: order.DateChangedMessage(mf), by: w.by.UserID(), at: w.now})
			if err != nil {
				return nil, err
			}
			return []appshared.Event{order.DateChanged{Order: o.ID(), From: old, To: in.Date}}, nil
		}})
	return MessagedView{Order: v, WhatsAppURL: wa}, err
}
