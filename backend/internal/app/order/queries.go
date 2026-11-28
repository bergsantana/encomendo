package order

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// maxQueryRunes is the longest search text List accepts (OpenAPI OrderSearch).
const maxQueryRunes = 100

// NumberQuery reads a search text as an order number: digits, optionally
// after "P-" in either case, such as "231" or "P-0231". Any other text, a
// name, is not a number. Repositories use it so the fake and Postgres agree
// on what a number search is.
func NumberQuery(q string) (order.Number, bool) {
	q = strings.TrimSpace(q)
	if len(q) >= 2 && (q[0] == 'P' || q[0] == 'p') && q[1] == '-' {
		q = q[2:]
	}
	if q == "" || len(q) > 9 {
		return 0, false
	}
	for _, r := range q {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(q)
	if err != nil || n < 1 {
		return 0, false
	}
	return order.Number(n), true
}

// normalized returns f ready for the repository: the default scope, a known
// scope and phase, and a trimmed query of a sane length.
func (f ListFilter) normalized() (ListFilter, error) {
	var bad []shared.FieldError
	switch f.Scope {
	case "":
		f.Scope = ScopeActive
	case ScopeActive, ScopeHandedOver, ScopeCancelled, ScopeAll:
	default:
		bad = append(bad, shared.FieldError{Field: "scope", Message: "Escolha ativos, entregues, cancelados ou todos."})
	}
	if f.Phase != "" {
		if _, err := workflow.ParsePhase(string(f.Phase)); err != nil {
			bad = append(bad, shared.FieldError{Field: "phase", Message: "Escolha uma fase válida."})
		}
	}
	f.Query = strings.TrimSpace(f.Query)
	if utf8.RuneCountInString(f.Query) > maxQueryRunes {
		bad = append(bad, shared.FieldError{Field: "q", Message: "Use uma busca de até 100 caracteres."})
	}
	if len(bad) > 0 {
		return ListFilter{}, shared.NewValidationError(bad...)
	}
	// Closed orders have no alerts, so "all" with the alert filter is
	// "active" and reads fewer rows.
	if f.Alert && f.Scope == ScopeAll {
		f.Scope = ScopeActive
	}
	return f, nil
}

// readCtx is what a query's body gets: who asks, the store's facts and its
// day.
type readCtx struct {
	by    shared.Principal
	facts StoreFacts
	now   time.Time
	today shared.CivilDate
}

// read runs a query: authorize first, then in one transaction find the
// store's day and call fn. Nothing a query reads is cached: orders and
// customer data never are (docs/architecture.md section 2).
func (s *Service) read(ctx context.Context, perm identity.Permission, fn func(ctx context.Context, r readCtx) error) error {
	by, err := authz.Require(ctx, perm)
	if err != nil {
		return err
	}
	return s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		facts, err := s.d.Store.Facts(ctx)
		if err != nil {
			return fmt.Errorf("store facts: %w", err)
		}
		now, today := s.now(facts)
		return fn(ctx, readCtx{by: by, facts: facts, now: now, today: today})
	})
}

// restoreAll rebuilds orders from their records, loading each workflow
// version once.
func (s *Service) restoreAll(ctx context.Context, recs []order.Record) ([]*order.Order, error) {
	type version struct {
		id shared.WorkflowID
		n  int
	}
	known := map[version]workflow.Workflow{}
	out := make([]*order.Order, 0, len(recs))
	for _, r := range recs {
		k := version{r.WorkflowID, r.WorkflowVersion}
		wf, ok := known[k]
		if !ok {
			var err error
			if wf, err = s.d.Workflows.Get(ctx, r.WorkflowID, r.WorkflowVersion); err != nil {
				return nil, fmt.Errorf("workflow of order %s: %w", r.ID, err)
			}
			known[k] = wf
		}
		o, err := order.Restore(r, wf)
		if err != nil {
			return nil, fmt.Errorf("restore order %s: %w", r.ID, err)
		}
		out = append(out, o)
	}
	return out, nil
}

// customerNames reads each customer's name once per query. The Customers
// port has no batch read, and a screen shows few distinct customers.
type customerNames struct {
	customers Customers
	known     map[shared.CustomerID]string
}

func newCustomerNames(c Customers) *customerNames {
	return &customerNames{customers: c, known: map[shared.CustomerID]string{}}
}

func (n *customerNames) of(ctx context.Context, id shared.CustomerID) (string, error) {
	if name, ok := n.known[id]; ok {
		return name, nil
	}
	c, err := n.customers.Get(ctx, id)
	if err != nil {
		return "", fmt.Errorf("customer %s: %w", id, err)
	}
	n.known[id] = c.Name
	return c.Name, nil
}

// summary builds o's summary line with its customer's name.
func (n *customerNames) summary(ctx context.Context, o *order.Order, today shared.CivilDate) (SummaryView, error) {
	name, err := n.of(ctx, o.Customer())
	if err != nil {
		return SummaryView{}, err
	}
	return toSummary(o, name, today), nil
}

// List returns one page of the store's orders that match f (order:read),
// by delivery date. Summaries carry no cost or margin for any role. The
// alert filter keeps active orders with an alert today: it reads further
// pages, up to alertPagesMax, until the page is full, so "Com alerta" is not
// an empty page with a cursor.
func (s *Service) List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[SummaryView], error) {
	var out shared.Page[SummaryView]
	err := s.read(ctx, identity.PermOrderRead, func(ctx context.Context, r readCtx) error {
		f, err := f.normalized()
		if err != nil {
			return err
		}
		names := newCustomerNames(s.d.Customers)
		for read := 0; read < alertPagesMax; read++ {
			page, err := s.d.Repo.List(ctx, r.by.TenantID(), f, p)
			if err != nil {
				return fmt.Errorf("list orders: %w", err)
			}
			if err := s.appendSummaries(ctx, &out, page.Items, f, names, r); err != nil {
				return err
			}
			out.NextCursor = page.NextCursor
			if !f.Alert || len(out.Items) >= p.Limit() || !page.HasMore() {
				return nil
			}
			if p, err = shared.NewPageRequest(page.NextCursor, p.Limit()); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// alertPagesMax bounds how many repository pages one alert-filtered List
// reads to fill its page.
const alertPagesMax = 5

// appendSummaries adds the summaries of recs that pass the alert filter.
func (s *Service) appendSummaries(ctx context.Context, out *shared.Page[SummaryView], recs []order.Record, f ListFilter, names *customerNames, r readCtx) error {
	orders, err := s.restoreAll(ctx, recs)
	if err != nil {
		return err
	}
	for _, o := range orders {
		if f.Alert && len(o.Alerts(r.today)) == 0 {
			continue
		}
		v, err := names.summary(ctx, o, r.today)
		if err != nil {
			return err
		}
		out.Items = append(out.Items, v)
	}
	return nil
}

// History returns one page of the order's history, newest first
// (order:read). A removed item has no name: the history names an item by
// what the order holds now.
func (s *Service) History(ctx context.Context, id shared.OrderID, p shared.PageRequest) (shared.Page[HistoryView], error) {
	var out shared.Page[HistoryView]
	err := s.read(ctx, identity.PermOrderRead, func(ctx context.Context, r readCtx) error {
		tenant := r.by.TenantID()
		rec, err := s.d.Repo.ByID(ctx, tenant, id)
		if err != nil {
			return fmt.Errorf("load order %s: %w", id, err)
		}
		page, err := s.d.Repo.History(ctx, tenant, id, p)
		if err != nil {
			return fmt.Errorf("history of order %s: %w", id, err)
		}
		names := itemNames(rec)
		for _, row := range page.Items {
			out.Items = append(out.Items, toHistoryView(row, names[row.Entry.Item]))
		}
		out.NextCursor = page.NextCursor
		return nil
	})
	return out, err
}

func itemNames(rec order.Record) map[shared.OrderItemID]string {
	out := map[shared.OrderItemID]string{}
	for _, l := range rec.Lines {
		for _, i := range l.Items {
			out[i.ID] = i.Snapshot.Name
		}
	}
	return out
}
