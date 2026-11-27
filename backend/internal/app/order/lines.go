package order

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// newLines resolves reqs against the catalog and gives every line and item
// a fresh id.
func (s *Service) newLines(ctx context.Context, reqs []LineRequest) ([]order.LineSpec, error) {
	specs, err := s.d.Catalog.OrderLines(ctx, reqs)
	if err != nil {
		return nil, err
	}
	for i := range specs {
		s.freshIDs(&specs[i])
	}
	return specs, nil
}

func (s *Service) freshIDs(l *order.LineSpec) {
	l.ID = shared.OrderLineID(s.d.IDs.NewID())
	for j := range l.Items {
		l.Items[j].ID = shared.OrderItemID(s.d.IDs.NewID())
	}
}

// changeLines resolves the lines of a change to o, so that later catalog
// edits never reprice what the client did not change (docs/architecture.md
// section 4.2, snapshots):
//
//   - a line sent again exactly as it is (same kit and swaps, or same item
//     and quantity, and the same choices) keeps its old price, snapshots,
//     sourcing and ids;
//   - any other line is priced by the catalog now. When it is the same kit
//     or loose item as an old line, the items it still holds keep their ids,
//     so their status, check and arrival survive (order.Change), and keep
//     their sourcing and lead days, so a backup-supplier switch survives.
func (s *Service) changeLines(ctx context.Context, o *order.Order, reqs []LineRequest) ([]order.LineSpec, error) {
	old := o.Lines()
	used := make([]bool, len(old))
	out := make([]order.LineSpec, len(reqs))
	var fresh []LineRequest
	var at []int
	for i, r := range reqs {
		if k := match(old, used, func(l order.Line) bool { return requestKey(r) == lineKey(l) }); k >= 0 {
			out[i] = specOf(old[k])
			continue
		}
		fresh, at = append(fresh, r), append(at, i)
	}
	if len(fresh) == 0 {
		return out, nil
	}
	specs, err := s.d.Catalog.OrderLines(ctx, fresh)
	if err != nil {
		return nil, err
	}
	for j, spec := range specs {
		r := fresh[j]
		if k := match(old, used, func(l order.Line) bool { return sameProduct(r, l) }); k >= 0 {
			s.keepIDs(&spec, old[k])
		} else {
			s.freshIDs(&spec)
		}
		out[at[j]] = spec
	}
	return out, nil
}

// match returns the first unused old line for which ok holds, marking it
// used, or -1.
func match(old []order.Line, used []bool, ok func(order.Line) bool) int {
	for k, l := range old {
		if !used[k] && ok(l) {
			used[k] = true
			return k
		}
	}
	return -1
}

// keepIDs gives spec the old line's id, and to each item of the same
// catalog item the old item's id, sourcing and lead days.
func (s *Service) keepIDs(spec *order.LineSpec, old order.Line) {
	spec.ID = old.ID()
	olds := old.Items()
	taken := make([]bool, len(olds))
	for j := range spec.Items {
		it := &spec.Items[j]
		k := -1
		for idx, o := range olds {
			if !taken[idx] && o.Snapshot().CatalogItem == it.Snapshot.CatalogItem {
				k = idx
				break
			}
		}
		if k < 0 {
			it.ID = shared.OrderItemID(s.d.IDs.NewID())
			continue
		}
		taken[k] = true
		it.ID = olds[k].ID()
		it.Sourcing = olds[k].Sourcing()
		it.Snapshot.LeadDays = olds[k].Snapshot().LeadDays
	}
}

// specOf turns an existing line back into a spec, unchanged.
func specOf(l order.Line) order.LineSpec {
	spec := order.LineSpec{ID: l.ID(), Price: l.Price()}
	if k, ok := l.Kit(); ok {
		spec.Kit = &k
	}
	spec.AssemblyDays, spec.SafetyMarginDays = l.KitDays()
	for _, i := range l.Items() {
		spec.Items = append(spec.Items, order.ItemSpec{ID: i.ID(), Snapshot: i.Snapshot(), Sourcing: i.Sourcing()})
	}
	return spec
}

// sameProduct reports whether r asks for the same kit or loose item as l.
func sameProduct(r LineRequest, l order.Line) bool {
	if k, ok := l.Kit(); ok {
		return r.Kit == k.Kit
	}
	items := l.Items()
	return r.Kit.IsZero() && len(items) == 1 && items[0].Snapshot().CatalogItem == r.Item
}

// requestKey and lineKey describe a line by what the client chose, so an
// unchanged line compares equal to its request.
func requestKey(r LineRequest) string {
	var swaps, opts []string
	for _, sw := range r.Swaps {
		swaps = append(swaps, sw.Item.String()+">"+sw.With.String())
	}
	for _, c := range r.Options {
		opts = append(opts, c.Item.String()+"/"+c.Option.String()+"="+c.Choice.String())
	}
	if !r.Kit.IsZero() {
		return key("kit", r.Kit.String(), 0, swaps, opts)
	}
	return key("item", r.Item.String(), r.Quantity, swaps, opts)
}

func lineKey(l order.Line) string {
	var swaps, opts []string
	for _, i := range l.Items() {
		snap := i.Snapshot()
		for _, c := range snap.Options {
			opts = append(opts, snap.CatalogItem.String()+"/"+c.Option.String()+"="+c.Choice.String())
		}
	}
	if k, ok := l.Kit(); ok {
		for _, sw := range k.Swaps {
			swaps = append(swaps, sw.Removed.String()+">"+sw.Added.String())
		}
		return key("kit", k.Kit.String(), 0, swaps, opts)
	}
	snap := l.Items()[0].Snapshot()
	return key("item", snap.CatalogItem.String(), snap.Quantity, swaps, opts)
}

func key(kind, id string, qty int, swaps, opts []string) string {
	slices.Sort(swaps)
	slices.Sort(opts)
	return fmt.Sprintf("%s:%s:%d:%s:%s", kind, id, qty, strings.Join(swaps, ","), strings.Join(opts, ","))
}
