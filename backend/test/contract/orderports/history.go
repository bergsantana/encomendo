package orderports

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

var historyCases = []contractCase{
	{"entries are appended in order and read back newest first with the user's name", historyAppended},
	{"entries with the same time keep the order they were written in", historySameTime},
	{"pages follow the cursor without gaps or repeats", historyPages},
	{"an entry with no user, or a user who is gone, has no name", historyUnnamed},
	{"an unknown order is not found", historyUnknownOrder},
	{"another store's order is not found", historyOtherStore},
}

// history reads up to limit entries of the order, newest first.
func (h Harness) history(t *testing.T, s store, id shared.OrderID, limit int) []apporder.HistoryRow {
	t.Helper()
	var rows []apporder.HistoryRow
	h.in(t, s, func(ctx context.Context) error {
		p, err := h.Orders.History(ctx, s.id, id, page(t, "", limit))
		rows = p.Items
		return err
	})
	return rows
}

func (h Harness) historyPage(t *testing.T, s store, id shared.OrderID, cursor string, limit int) shared.Page[apporder.HistoryRow] {
	t.Helper()
	var p shared.Page[apporder.HistoryRow]
	h.in(t, s, func(ctx context.Context) error {
		var err error
		p, err = h.Orders.History(ctx, s.id, id, page(t, cursor, limit))
		return err
	})
	return p
}

func historyAppended(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	want := o.PendingHistory()
	loaded := h.load(t, s, o.ID())
	recordDeposit(t, h, s, loaded, epoch.Add(time.Hour))
	want = append(want, loaded.PendingHistory()...)
	h.mustUpdate(t, s, loaded)
	loaded = h.load(t, s, o.ID())
	awaiting := h.status(t, workflow.PhaseAwaitingDeposit).Key
	if err := loaded.Advance(awaiting, nil, s.principal(t), today(epoch), epoch.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	want = append(want, loaded.PendingHistory()...)
	h.mustUpdate(t, s, loaded)

	rows := h.history(t, s, o.ID(), 50)
	if len(rows) != len(want) {
		t.Fatalf("history has %d entries, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		sameRecord(t, row.Entry, want[len(want)-1-i])
		if row.ByName != "Ana Souza" {
			t.Errorf("entry %d by %q, want the user's name", i, row.ByName)
		}
	}
}

func historySameTime(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	loaded := h.load(t, s, o.ID())
	awaiting := h.status(t, workflow.PhaseAwaitingDeposit).Key
	if err := loaded.Advance(awaiting, nil, s.principal(t), today(epoch), epoch); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, s, loaded)
	rows := h.history(t, s, o.ID(), 50)
	if len(rows) != 2 || rows[0].Entry.Kind != order.HistoryStatus || rows[1].Entry.Kind != order.HistoryCreated {
		t.Errorf("history = %+v, want [status created]", rows)
	}
}

func historyPages(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	for i := range 4 {
		loaded := h.load(t, s, o.ID())
		recordSmall(t, h, s, loaded, epoch.Add(time.Duration(i+1)*time.Hour))
		h.mustUpdate(t, s, loaded)
	}
	all := h.history(t, s, o.ID(), 50)
	if len(all) != 5 {
		t.Fatalf("history has %d entries, want 5", len(all))
	}
	var got []apporder.HistoryRow
	cursor, pages := "", 0
	for pages < 6 {
		p := h.historyPage(t, s, o.ID(), cursor, 2)
		got = append(got, p.Items...)
		pages++
		if cursor = p.NextCursor; cursor == "" {
			break
		}
	}
	if pages != 3 || !reflect.DeepEqual(got, all) {
		t.Errorf("paged %d pages to %d entries; want 3 pages equal to the full read (%d entries)", pages, len(got), len(all))
	}
}

func historyUnnamed(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s, anonymous())
	loaded := h.load(t, s, o.ID())
	stranger, err := shared.NewBackofficePrincipal(shared.UserID(h.NewID()), s.id, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Cancel("Desistiu", stranger, epoch.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, s, loaded)
	rows := h.history(t, s, o.ID(), 10)
	if len(rows) != 2 {
		t.Fatalf("history has %d entries, want 2", len(rows))
	}
	for i, row := range rows {
		if row.ByName != "" {
			t.Errorf("entry %d (%s) by %q, want no name", i, row.Entry.Kind, row.ByName)
		}
	}
	if !rows[1].Entry.By.IsZero() || rows[0].Entry.By.IsZero() {
		t.Errorf("by = %v and %v, want the cancel by the stranger and the creation by nobody", rows[0].Entry.By, rows[1].Entry.By)
	}
}

func (h Harness) historyErr(s store, id shared.OrderID) error {
	return h.try(s.scope(), func(ctx context.Context) error {
		_, err := h.Orders.History(ctx, s.id, id, shared.PageRequest{})
		return err
	})
}

func historyUnknownOrder(t *testing.T, h Harness) {
	s := h.newStore(t)
	if err := h.historyErr(s, shared.OrderID(h.NewID())); !errors.Is(err, apporder.ErrNotFound) {
		t.Errorf("History err = %v, want apporder.ErrNotFound", err)
	}
}

func historyOtherStore(t *testing.T, h Harness) {
	s, other := h.newStore(t), h.newStore(t)
	o := h.insert(t, s)
	if err := h.historyErr(other, o.ID()); !errors.Is(err, apporder.ErrNotFound) {
		t.Errorf("History in another store err = %v, want apporder.ErrNotFound", err)
	}
}
