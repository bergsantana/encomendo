package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// linkPlaceholder replaces the tracking link in every stored message, so
// order_messages never holds a live token (ADR 0030, Amendments).
const linkPlaceholder = "[link de acompanhamento]"

// MessagedView is an order with the wa.me link of the message made for it.
type MessagedView struct {
	Order       View
	WhatsAppURL string
}

// LinkView is a new tracking link and the wa.me link that sends it.
type LinkView struct {
	TrackingURL string
	WhatsAppURL string
}

// outgoing is one message to make for an order.
type outgoing struct {
	order *order.Order
	to    CustomerFacts
	kind  MessageKind
	text  string
	link  string // the tracking link the text carries, or ""
	by    shared.UserID
	at    time.Time
}

// message builds the wa.me link of m's text for the user to send, and
// records the message with the tracking link replaced. Nothing is sent
// (docs/architecture.md section 2, rule 9).
func (s *Service) message(ctx context.Context, m outgoing) (string, error) {
	url, err := s.d.WhatsApp.Link(m.to.Phone, m.text)
	if err != nil {
		return "", fmt.Errorf("wa.me link: %w", err)
	}
	stored := m.text
	if m.link != "" {
		stored = strings.ReplaceAll(stored, m.link, linkPlaceholder)
	}
	storedURL, err := s.d.WhatsApp.Link(m.to.Phone, stored)
	if err != nil {
		return "", fmt.Errorf("wa.me link: %w", err)
	}
	err = s.d.Repo.AddMessage(ctx, Message{
		ID: s.d.IDs.NewID(), Order: m.order.ID(), Kind: m.kind, ToPhone: m.to.Phone,
		Text: stored, URL: storedURL, By: m.by, At: m.at,
	})
	if err != nil {
		return "", fmt.Errorf("record message: %w", err)
	}
	return url, nil
}

// messageFacts is what the WhatsApp texts may say of o: no supplier, cost
// or alert, since MessageFacts has no field for them.
func messageFacts(f StoreFacts, c CustomerFacts, o *order.Order) order.MessageFacts {
	total, _ := o.Total() // New and Change refuse totals that overflow
	return order.MessageFacts{
		Store: f.Name, CustomerFirstName: customer.FirstName(c.Name), Number: o.Number().String(),
		Total: total, Deposit: o.DepositDue(), Refund: o.RefundDue(), NewDate: o.Promise(),
	}
}

// SendTrackingLink issues a new tracking link for the order (order:write)
// and the wa.me link that sends it. The previous link stops working, since
// only the hash of the newest is kept (ADR 0030).
func (s *Service) SendTrackingLink(ctx context.Context, id shared.OrderID) (LinkView, error) {
	var out LinkView
	_, err := s.write(ctx, write{
		perm: identity.PermOrderWrite, order: id, anyVersion: true,
		change: func(ctx context.Context, o *order.Order, w writeCtx) ([]appshared.Event, error) {
			token, err := s.d.Secrets.NewSecret(ctx)
			if err != nil {
				return nil, fmt.Errorf("tracking token: %w", err)
			}
			o.RotateTracking(token.Hash(), w.by, w.now)
			link, err := s.d.Tracking.URL(w.facts.Slug, token)
			if err != nil {
				return nil, fmt.Errorf("tracking url: %w", err)
			}
			c, err := s.d.Customers.Get(ctx, o.Customer())
			if err != nil {
				return nil, err
			}
			text := order.TrackingMessage(messageFacts(w.facts, c, o), link)
			wa, err := s.message(ctx, outgoing{order: o, to: c, kind: MessageTrackingLink, text: text, link: link, by: w.by.UserID(), at: w.now})
			if err != nil {
				return nil, err
			}
			out = LinkView{TrackingURL: link, WhatsAppURL: wa}
			return nil, nil
		},
	})
	return out, err
}
