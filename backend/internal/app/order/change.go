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

// ChangedView is a changed order, what the change did, and, when the order
// was paid, the wa.me link of the summary for the customer.
type ChangedView struct {
	Order       View
	Summary     order.ChangeResult
	WhatsAppURL string
}

// Change replaces an order's lines and terms (ADR 0030). Before any payment
// it needs order:write; once paid it needs order:manage and the seller's
// confirmation that the customer agreed (order.ErrApprovalRequired), and
// answers the wa.me summary. A discount, deposit or days different from the
// order's own need order:manage either way. Lines sent back unchanged keep
// their prices and snapshots (changeLines). It returns the domain's
// refusals (order.ErrNotEditable, order.ErrOrderClosed), shared.ErrForbidden
// and shared.ErrVersionConflict.
func (s *Service) Change(ctx context.Context, in ChangeInput) (ChangedView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return ChangedView{}, err
	}
	var out ChangedView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		f, err := s.d.Store.Facts(ctx)
		if err != nil {
			return fmt.Errorf("store facts: %w", err)
		}
		o, err := s.load(ctx, by.TenantID(), in.Order)
		if err != nil {
			return err
		}
		if err := o.CheckVersion(in.Version); err != nil {
			return err
		}
		out, err = s.change(ctx, o, in, by, f)
		return err
	})
	return out, err
}

func (s *Service) change(ctx context.Context, o *order.Order, in ChangeInput, by shared.Principal, f StoreFacts) (ChangedView, error) {
	now, today := s.now(f)
	res, err := s.applyChange(ctx, o, in, by, f)
	if err != nil {
		return ChangedView{}, err
	}
	err = s.save(ctx, o, []appshared.Event{order.Changed{Order: o.ID(), Added: res.Added, Removed: res.Removed}})
	if err != nil {
		return ChangedView{}, err
	}
	c, err := s.d.Customers.Get(ctx, o.Customer())
	if err != nil {
		return ChangedView{}, err
	}
	v := toView(o, c, by, today)
	v.Version = o.Version() + 1 // the repository stored the next version
	out := ChangedView{Order: v, Summary: res}
	if res.Paid {
		text := order.ChangedMessage(messageFacts(f, c, o))
		out.WhatsAppURL, err = s.message(ctx, outgoing{order: o, to: c, kind: MessageChanged, text: text, by: by.UserID(), at: now})
	}
	return out, err
}

// applyChange checks who may change o, resolves the lines and terms and
// changes o in memory. Quote uses it on an order it never saves.
func (s *Service) applyChange(ctx context.Context, o *order.Order, in ChangeInput, by shared.Principal, f StoreFacts) (order.ChangeResult, error) {
	now, today := s.now(f)
	if o.Editable() == order.EditApproval {
		if err := requireManage(by, "change a paid order"); err != nil {
			return order.ChangeResult{}, err
		}
	}
	c, err := s.d.Customers.Get(ctx, o.Customer())
	if err != nil {
		return order.ChangeResult{}, fmt.Errorf("customer of order %s: %w", o.ID(), err)
	}
	lines, err := s.changeLines(ctx, o, in.Lines)
	if err != nil {
		return order.ChangeResult{}, err
	}
	terms, err := resolveTerms(by, o.Terms(), in.Terms, c)
	if err != nil {
		return order.ChangeResult{}, err
	}
	return o.Change(order.ChangeSpec{
		Lines: lines, Terms: terms, CustomerApproved: in.CustomerApproved, By: by, Now: now, Today: today,
	})
}
