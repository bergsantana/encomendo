package order

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// TrackingView is what the client sees on the tracking page (milestone 7
// serves it). It is its own type, never View with fields left out, and
// it has no field that could carry a supplier, a cost, a margin, an alert,
// an expected date, a staff name or the history (docs/architecture.md
// section 2, rule 8). Its date is the promise, never the forecast: a later
// forecast is "Pode atrasar", which the client never sees.
type TrackingView struct {
	Store        string
	Number       string
	Steps        []TrackingStep
	Cancelled    bool
	DeliveryDate shared.CivilDate
	Items        []TrackingItem
	Total        shared.Money
	Paid         shared.Money
	BalanceDue   shared.Money
	Handover     order.Handover
}

// TrackingStep is one status of the order's workflow, by its client label.
type TrackingStep struct {
	Label   string
	Current bool
}

// TrackingItem is one item in plain words: "Pronto" when Arrived, "A
// caminho" otherwise.
type TrackingItem struct {
	Name     string
	Quantity int
	Options  []string // "Cor da costura: Vermelha"
	Arrived  bool
}

// NewTrackingView builds the client's view of o, in the store named store.
// The steps are the client labels of the order's own workflow version; a
// cancelled order shows one step.
func NewTrackingView(o *order.Order, store string) TrackingView {
	total, _ := o.Total() // New and Change refuse totals that overflow
	v := TrackingView{
		Store: store, Number: o.Number().String(), DeliveryDate: o.Promise(),
		Total: total, Paid: o.Paid(), BalanceDue: o.BalanceDue(), Handover: o.Terms().Handover,
	}
	if o.Phase() == workflow.PhaseCancelled {
		v.Cancelled = true
		v.Steps = []TrackingStep{{Label: workflow.Cancelled.ClientLabel, Current: true}}
	} else {
		for _, s := range o.Workflow().Statuses() {
			v.Steps = append(v.Steps, TrackingStep{Label: s.ClientLabel, Current: s.Key == o.Status().Key})
		}
	}
	for _, i := range o.Items() {
		snap := i.Snapshot()
		ti := TrackingItem{Name: snap.Name, Quantity: snap.Quantity, Arrived: i.Arrived()}
		for _, opt := range snap.Options {
			ti.Options = append(ti.Options, opt.Name+": "+opt.Label)
		}
		v.Items = append(v.Items, ti)
	}
	return v
}
