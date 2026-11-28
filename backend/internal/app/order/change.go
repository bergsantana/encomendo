package order

import (
	"context"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
)

// ChangedView is a changed order, what the change did, and, when the order
// was paid, the wa.me link of the summary for the customer.
type ChangedView struct {
	Order       View
	Summary     order.ChangeResult
	WhatsAppURL string
}

// Change replaces an order's lines and terms (ADR 0030 and its
// Amendments):
//
//   - before any payment it needs order:write, unless the order has a
//     discount: then any change needs order:manage, so staff cannot carry
//     an owner's or admin's discount over to another basket;
//   - once paid it needs order:manage and the seller's confirmation that the
//     customer agreed (order.ErrApprovalRequired), and answers the wa.me
//     summary;
//   - setting a discount, deposit or days different from the order's own
//     needs order:manage either way.
//
// Lines sent back unchanged keep their prices and snapshots (changeLines).
// It returns the domain's refusals (order.ErrNotEditable,
// order.ErrOrderClosed), shared.ErrForbidden and shared.ErrVersionConflict.
func (s *Service) Change(ctx context.Context, in ChangeInput) (ChangedView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return ChangedView{}, err
	}
	var out ChangedView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		o, err := s.load(ctx, by.TenantID(), in.Order)
		if err != nil {
			return err
		}
		if err := o.CheckVersion(in.Version); err != nil {
			return err
		}
		out, err = s.change(ctx, o, in, w)
		return err
	})
	return out, err
}

func (s *Service) change(ctx context.Context, o *order.Order, in ChangeInput, w writeCtx) (ChangedView, error) {
	res, err := s.applyChange(ctx, o, in, w)
	if err != nil {
		return ChangedView{}, err
	}
	err = s.save(ctx, o, []appshared.Event{order.Changed{Order: o.ID(), Added: res.Added, Removed: res.Removed}})
	if err != nil {
		return ChangedView{}, err
	}
	v, err := s.savedView(ctx, o, w.by, w.today)
	if err != nil {
		return ChangedView{}, err
	}
	out := ChangedView{Order: v, Summary: res}
	if res.Paid {
		c, err := s.customerOf(ctx, o)
		if err != nil {
			return ChangedView{}, err
		}
		text := order.ChangedMessage(messageFacts(w.facts, c, o))
		out.WhatsAppURL, err = s.message(ctx, outgoing{order: o, to: c, kind: MessageChanged, text: text, by: w.by.UserID(), at: w.now})
		if err != nil {
			return ChangedView{}, err
		}
	}
	return out, nil
}

// applyChange checks who may change o, resolves the lines and terms and
// changes o in memory. Quote uses it on an order it never saves.
func (s *Service) applyChange(ctx context.Context, o *order.Order, in ChangeInput, w writeCtx) (order.ChangeResult, error) {
	if err := s.mayChange(o, w); err != nil {
		return order.ChangeResult{}, err
	}
	c, err := s.customerOf(ctx, o)
	if err != nil {
		return order.ChangeResult{}, err
	}
	lines, err := s.changeLines(ctx, o, in.Lines)
	if err != nil {
		return order.ChangeResult{}, err
	}
	terms, err := resolveTerms(w.by, o.Terms(), in.Terms, c)
	if err != nil {
		return order.ChangeResult{}, err
	}
	return o.Change(order.ChangeSpec{
		Lines: lines, Terms: terms, CustomerApproved: in.CustomerApproved, By: w.by, Now: w.now, Today: w.today,
	})
}

// mayChange refuses a change of a paid order, or of a discounted one, to a
// role without order:manage (ADR 0030, Amendments). Only the discount
// locks the lines: it is never a default, so it always means an owner or
// admin set it. A deposit or days other than the store's are not enough:
// they drift from the defaults on their own (a kit order changed to loose
// items keeps its kit's days; the owner edits the store's defaults), and
// staff still cannot change them (resolveTerms).
func (s *Service) mayChange(o *order.Order, w writeCtx) error {
	if o.Editable() == order.EditApproval {
		return requireManage(w.by, "change a paid order")
	}
	if !o.Terms().Discount.IsZero() {
		return requireManage(w.by, "change a discounted order")
	}
	return nil
}
