package fake

import (
	"cmp"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"slices"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Link gives the repository what Postgres joins in SQL: the workflows, to
// find the phase of a stored status key, and the customers, to find a name.
// A filter by phase or by name fails until it is called.
func (r *Orders) Link(w apporder.Workflows, c apporder.Customers) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflows, r.customers = w, c
}

// NameUser sets the name History shows for a user.
func (r *Orders) NameUser(id shared.UserID, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.userNames == nil {
		r.userNames = map[shared.UserID]string{}
	}
	r.userNames[id] = name
}

// scopeRows returns the committed rows of tenant, by (delivery date, id).
// The caller holds the lock.
func (r *Orders) scopeRows(tenant shared.TenantID) []order.Record {
	var out []order.Record
	for k, rec := range r.rows {
		if k.tenant == tenant {
			out = append(out, rec)
		}
	}
	slices.SortFunc(out, func(a, b order.Record) int { return cmp.Compare(deliveryKey(a), deliveryKey(b)) })
	return out
}

func deliveryKey(rec order.Record) string { return rec.Promise.String() + "|" + rec.ID.String() }

func closed(rec order.Record) bool { return !rec.CancelledAt.IsZero() || !rec.HandedOverAt.IsZero() }

// matches reports whether rec passes f, as the SQL of the Postgres
// repository would. The caller holds the lock.
func (r *Orders) matches(ctx context.Context, rec order.Record, f apporder.ListFilter) (bool, error) {
	switch f.Scope {
	case apporder.ScopeActive:
		if closed(rec) {
			return false, nil
		}
	case apporder.ScopeHandedOver:
		if rec.HandedOverAt.IsZero() {
			return false, nil
		}
	case apporder.ScopeCancelled:
		if rec.CancelledAt.IsZero() {
			return false, nil
		}
	}
	if !f.Customer.IsZero() && rec.Customer != f.Customer {
		return false, nil
	}
	if f.Status != "" || f.Phase != "" {
		ok, err := r.statusMatches(ctx, rec, f)
		if !ok || err != nil {
			return false, err
		}
	}
	if f.Query == "" {
		return true, nil
	}
	if n, ok := apporder.NumberQuery(f.Query); ok {
		return rec.Number == n, nil
	}
	if r.customers == nil {
		return false, errors.New("fake: a name search needs Link")
	}
	c, err := r.customers.Get(ctx, rec.Customer)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(c.Name), strings.ToLower(f.Query)), nil
}

// statusMatches checks the status and phase filters. A cancelled order's
// status is the workflow's implicit cancelled one.
func (r *Orders) statusMatches(ctx context.Context, rec order.Record, f apporder.ListFilter) (bool, error) {
	st := workflow.Cancelled
	if rec.CancelledAt.IsZero() {
		if r.workflows == nil {
			return false, errors.New("fake: a status filter needs Link")
		}
		wf, err := r.workflows.Get(ctx, rec.WorkflowID, rec.WorkflowVersion)
		if err != nil {
			return false, err
		}
		var ok bool
		if st, ok = wf.Status(rec.StatusKey); !ok {
			return false, errors.New("fake: stored status is not in its workflow")
		}
	}
	return (f.Status == "" || st.Key == f.Status) && (f.Phase == "" || st.Phase == f.Phase), nil
}

// List implements apporder.Repository. Its cursor is the delivery date and
// id of the last order of the previous page.
func (r *Orders) List(ctx context.Context, tenant shared.TenantID, f apporder.ListFilter, p shared.PageRequest) (shared.Page[order.Record], error) {
	if err := r.enter(ctx, tenant); err != nil {
		return shared.Page[order.Record]{}, err
	}
	defer r.mu.Unlock()
	var page shared.Page[order.Record]
	for _, rec := range r.scopeRows(tenant) {
		if p.Cursor() != "" && deliveryKey(rec) <= p.Cursor() {
			continue
		}
		ok, err := r.matches(ctx, rec, f)
		if err != nil {
			return shared.Page[order.Record]{}, err
		}
		if !ok {
			continue
		}
		if len(page.Items) == p.Limit() {
			page.NextCursor = deliveryKey(page.Items[len(page.Items)-1])
			break
		}
		page.Items = append(page.Items, rec)
	}
	return page, nil
}

// Active implements apporder.Repository. A status move in the history sets
// StatusSince; an order that never moved has been in its status since it
// was created.
func (r *Orders) Active(ctx context.Context, tenant shared.TenantID, limit int) ([]apporder.ActiveOrder, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	var out []apporder.ActiveOrder
	for _, rec := range r.scopeRows(tenant) {
		if closed(rec) {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, apporder.ActiveOrder{Record: rec, StatusSince: r.statusSince(rec)})
	}
	return out, nil
}

func (r *Orders) statusSince(rec order.Record) time.Time {
	since := rec.CreatedAt
	for _, e := range r.history[orderKey{rec.Tenant, rec.ID}] {
		if e.Kind == order.HistoryStatus && e.To.Key == rec.StatusKey {
			since = e.At
		}
	}
	return since
}

// History implements apporder.Repository. Its cursor is the number of
// entries already returned.
func (r *Orders) History(ctx context.Context, tenant shared.TenantID, id shared.OrderID, p shared.PageRequest) (shared.Page[apporder.HistoryRow], error) {
	if err := r.enter(ctx, tenant); err != nil {
		return shared.Page[apporder.HistoryRow]{}, err
	}
	defer r.mu.Unlock()
	k := orderKey{tenant, id}
	if _, ok := r.rows[k]; !ok {
		return shared.Page[apporder.HistoryRow]{}, apporder.ErrNotFound
	}
	skip := 0
	if p.Cursor() != "" {
		n, err := strconv.Atoi(p.Cursor())
		if err != nil || n < 0 {
			return shared.Page[apporder.HistoryRow]{}, errors.New("fake: bad history cursor")
		}
		skip = n
	}
	all := slices.Clone(r.history[k])
	slices.Reverse(all) // newest first; the fake appends in order
	var page shared.Page[apporder.HistoryRow]
	for i := skip; i < len(all); i++ {
		if len(page.Items) == p.Limit() {
			page.NextCursor = strconv.Itoa(i)
			break
		}
		page.Items = append(page.Items, apporder.HistoryRow{Entry: all[i], ByName: r.userNames[all[i].By]})
	}
	return page, nil
}

// Payments implements apporder.Repository.
func (r *Orders) Payments(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]apporder.PaymentRow, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	var out []apporder.PaymentRow
	for _, rec := range r.scopeRows(tenant) {
		for _, p := range rec.Payments {
			if p.At.Before(from) || !p.At.Before(to) {
				continue
			}
			out = append(out, apporder.PaymentRow{
				Order: rec.ID, Kind: p.Kind, Method: p.Method, Amount: p.Amount, At: p.At, Voided: !p.VoidedAt.IsZero(),
			})
		}
	}
	slices.SortStableFunc(out, func(a, b apporder.PaymentRow) int { return a.At.Compare(b.At) })
	return out, nil
}

// HandedOver implements apporder.Repository.
func (r *Orders) HandedOver(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]order.Record, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	var out []order.Record
	for _, rec := range r.scopeRows(tenant) {
		if !rec.HandedOverAt.IsZero() && !rec.HandedOverAt.Before(from) && rec.HandedOverAt.Before(to) {
			out = append(out, rec)
		}
	}
	slices.SortStableFunc(out, func(a, b order.Record) int { return a.HandedOverAt.Compare(b.HandedOverAt) })
	return out[:min(len(out), apporder.HandedOverCap)], nil
}
