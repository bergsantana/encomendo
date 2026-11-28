package dto

import (
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// TrackingOrder is the TrackingOrder schema: what the client sees on the
// tracking page. It has no field for a supplier, a cost, a margin, an
// alert, a staff name or the history, and none may be added (docs/product/
// design-document.md, System rules). Nothing serves it before milestone 7;
// a test keeps its shape closed.
type TrackingOrder struct {
	Store        string           `json:"store"`
	Number       string           `json:"number"`
	Steps        []TrackingStep   `json:"steps"`
	Cancelled    bool             `json:"cancelled"`
	DeliveryDate shared.CivilDate `json:"deliveryDate"`
	Items        []TrackingItem   `json:"items"`
	Total        shared.Money     `json:"total"`
	Paid         shared.Money     `json:"paid"`
	BalanceDue   shared.Money     `json:"balanceDue"`
	Handover     order.Handover   `json:"handover"`
}

// TrackingStep is one status by its client label.
type TrackingStep struct {
	Label   string `json:"label"`
	Current bool   `json:"current"`
}

// TrackingItem is one item in plain words.
type TrackingItem struct {
	Name     string   `json:"name"`
	Quantity int      `json:"quantity"`
	Options  []string `json:"options"`
	Arrived  bool     `json:"arrived"`
}

// NewTrackingOrder maps the client's view of an order.
func NewTrackingOrder(v apporder.TrackingView) TrackingOrder {
	out := TrackingOrder{
		Store: v.Store, Number: v.Number, Cancelled: v.Cancelled, DeliveryDate: v.DeliveryDate,
		Total: v.Total, Paid: v.Paid, BalanceDue: v.BalanceDue, Handover: v.Handover,
		Steps: make([]TrackingStep, 0, len(v.Steps)), Items: make([]TrackingItem, 0, len(v.Items)),
	}
	for _, s := range v.Steps {
		out.Steps = append(out.Steps, TrackingStep{Label: s.Label, Current: s.Current})
	}
	for _, i := range v.Items {
		out.Items = append(out.Items, TrackingItem{
			Name: i.Name, Quantity: i.Quantity, Options: append([]string{}, i.Options...), Arrived: i.Arrived,
		})
	}
	return out
}
