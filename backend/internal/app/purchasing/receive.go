package purchasing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// receiptOutcome is a receipt the call answers: its view, without the
// completed orders, which are read after the transaction, and the orders it
// touched.
type receiptOutcome struct {
	view   ReceiptView
	orders []shared.OrderID
}

// Receive saves one receipt (receiving:write): each entry checks an open
// line "Tudo certo" or "Com defeito" (a defect needs a note), or an item
// still in "A comprar" that was bought at the counter, for which a line is
// written as it is received. A defect leaves the item in "A comprar" again,
// marked "Reposição". Lines of a cancelled order or of a removed item are
// received but their items are not moved (ADR 0031). Photos are optional:
// up to 4 of the invoice, 2 per entry, 20 in all.
//
// The Idempotency-Key is checked after the permission. A replay answers the
// receipt it first made, whatever state the lines are in now, and touches no
// storage. Photos are checked against storage before the transaction opens,
// so storage never holds one, and promoted last inside it (ADR 0028).
// CompletedOrders and BackToBuy are read after the commit, so a replay
// shows them as the orders stand now, and a failure of that read answers an
// error although the receipt is saved (a retry replays it).
//
// It returns ErrIdempotencyKeyRequired, appshared.ErrIdempotencyKeyReused,
// ErrItemNotToBuy for a counter item no longer in "A comprar", the domain's
// refusals (purchasing.ErrEmptyReceipt, ErrLineTwice, ErrPhotoLimit,
// ErrPhotoKey, ErrLineClosed, ErrLineNotFound), catalog.ErrPhotoNotUploaded
// and catalog.ErrPhotoInvalid for a bad photo, and the order context's
// refusals.
func (s *Service) Receive(ctx context.Context, in ReceiptInput) (ReceiptView, error) {
	by, err := authz.Require(ctx, identity.PermReceivingWrite)
	if err != nil {
		return ReceiptView{}, err
	}
	if in.Idempotency.IsZero() {
		return ReceiptView{}, ErrIdempotencyKeyRequired
	}
	// A retry of a saved receipt must not need its staged photos again.
	out, ok, err := s.recalled(ctx, by, in.Idempotency)
	if err != nil {
		return ReceiptView{}, err
	}
	if !ok {
		plan, err := s.planReceipt(ctx, by, in)
		if err != nil {
			return ReceiptView{}, err
		}
		if out, err = s.saveReceipt(ctx, by, in.Idempotency, plan); err != nil {
			return ReceiptView{}, err
		}
	}
	return s.completed(ctx, out)
}

// recalled runs the replay check in a transaction of its own.
func (s *Service) recalled(ctx context.Context, by shared.Principal, k appshared.IdempotencyKey) (receiptOutcome, bool, error) {
	var out receiptOutcome
	var ok bool
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		out, ok, err = s.replayReceipt(ctx, by, k)
		return err
	})
	return out, ok, err
}

// replayReceipt answers a key already used: the receipt it made, as its
// lines are now. A key that made anything else was reused for another
// request.
func (s *Service) replayReceipt(ctx context.Context, by shared.Principal, k appshared.IdempotencyKey) (receiptOutcome, bool, error) {
	resource, ok, err := s.d.Idempotency.Recall(ctx, k)
	if err != nil || !ok {
		return receiptOutcome{}, false, err
	}
	id, err := shared.ParseReceiptID(resource)
	if err != nil {
		return receiptOutcome{}, false, appshared.ErrIdempotencyKeyReused
	}
	rec, err := s.d.Repo.Receipt(ctx, by.TenantID(), id)
	if errors.Is(err, ErrReceiptNotFound) {
		return receiptOutcome{}, false, appshared.ErrIdempotencyKeyReused
	}
	if err != nil {
		return receiptOutcome{}, false, fmt.Errorf("read receipt %s: %w", id, err)
	}
	orders := make([]shared.OrderID, len(rec.Lines))
	for i, l := range rec.Lines {
		orders[i] = l.Item.Order
	}
	f, err := s.facts(ctx, orders...)
	if err != nil {
		return receiptOutcome{}, false, err
	}
	return receiptOutcome{view: receiptView(rec.ID, rec.At, rec.Lines, f), orders: distinct(orders)}, true, nil
}

// entryPlan is one entry of a receipt as the call prepared it.
type entryPlan struct {
	in     ReceiptEntryInput
	line   shared.PurchaseLineID // the line checked; new for a counter entry
	photos []purchasing.Photo
}

func (e entryPlan) counter() bool { return e.in.Line.IsZero() }

// receiptPlan is what Receive prepares before its transaction: the ids,
// and the photos checked against storage.
type receiptPlan struct {
	id      shared.ReceiptID
	entries []entryPlan
	invoice []purchasing.Photo
	staged  []staged // every photo, in the order to promote them
}

// spec is the receipt as the domain validates it.
func (p receiptPlan) spec(c call) purchasing.ReceiptSpec {
	entries := make([]purchasing.ReceiptEntry, len(p.entries))
	for i, e := range p.entries {
		entries[i] = purchasing.ReceiptEntry{Line: e.line, Check: e.in.Check, Note: e.in.Note, Photos: e.photos}
	}
	return purchasing.ReceiptSpec{
		Tenant: c.by.TenantID(), ID: p.id, By: c.by, Now: c.now, Entries: entries, Invoice: p.invoice,
	}
}

// planReceipt prepares a receipt without a transaction. It gives each
// counter entry its line id, then lets the domain validate the whole
// receipt with placeholder photos, so a request over the limits, with a
// foreign key or a bad note is refused before any storage call; only then
// does it check each upload against storage.
func (s *Service) planReceipt(ctx context.Context, by shared.Principal, in ReceiptInput) (receiptPlan, error) {
	plan := receiptPlan{id: shared.ReceiptID(s.d.IDs.NewID()), entries: make([]entryPlan, len(in.Entries))}
	counters := map[shared.OrderItemID]bool{}
	for i, e := range in.Entries {
		switch {
		case e.Line.IsZero() == e.OrderItem.IsZero():
			return receiptPlan{}, invalid(fmt.Sprintf("entries[%d]", i), msgEntryTarget)
		case e.Line.IsZero() && counters[e.OrderItem]:
			return receiptPlan{}, refuse(purchasing.ErrLineTwice, fmt.Sprintf("entries[%d].orderItemId", i), msgLineRepeated)
		}
		plan.entries[i] = entryPlan{in: e, line: e.Line}
		if e.Line.IsZero() {
			counters[e.OrderItem] = true
			plan.entries[i].line = shared.PurchaseLineID(s.d.IDs.NewID())
		}
	}
	tenant := by.TenantID()
	dry := plan
	dry.entries = slices.Clone(plan.entries)
	for i, e := range in.Entries {
		dry.entries[i].photos = placeholders(tenant, e.PhotoKeys)
	}
	dry.invoice = placeholders(tenant, in.InvoicePhotoKeys)
	if _, err := purchasing.NewReceipt(dry.spec(call{by: by, now: s.d.Clock.Now()})); err != nil {
		return receiptPlan{}, err
	}
	return plan, s.stagePhotos(ctx, tenant, &plan, in)
}

// placeholders stands in for photos not yet checked, so the domain can
// count and shape-check keys. A key that is not this store's gets the zero
// id, which NewReceipt refuses as purchasing.ErrPhotoKey.
func placeholders(tenant shared.TenantID, keys []string) []purchasing.Photo {
	out := make([]purchasing.Photo, len(keys))
	for i, k := range keys {
		id, _ := purchasing.ParsePhotoKey(tenant, k) // the error is the zero id, refused by NewReceipt
		out[i] = purchasing.Photo{ID: id, Key: k, ContentType: "image/jpeg", Size: 1}
	}
	return out
}

// stagePhotos checks every upload of in against storage, entries first,
// then the invoice, and records the real photos in plan.
func (s *Service) stagePhotos(ctx context.Context, tenant shared.TenantID, plan *receiptPlan, in ReceiptInput) error {
	for i, e := range in.Entries {
		for _, key := range e.PhotoKeys {
			st, err := s.checkUpload(ctx, tenant, key)
			if err != nil {
				return err
			}
			plan.entries[i].photos = append(plan.entries[i].photos, st.photo)
			plan.staged = append(plan.staged, st)
		}
	}
	for _, key := range in.InvoicePhotoKeys {
		st, err := s.checkUpload(ctx, tenant, key)
		if err != nil {
			return err
		}
		plan.invoice = append(plan.invoice, st.photo)
		plan.staged = append(plan.staged, st)
	}
	return nil
}

// saveReceipt writes the receipt in one transaction: the lines, the receipt
// and the item moves commit together or not at all. The photos are promoted
// last, so nothing but the commit can fail after a copy; a copy that may
// have been made when the transaction fails is reported as an orphan.
func (s *Service) saveReceipt(ctx context.Context, by shared.Principal, k appshared.IdempotencyKey, plan receiptPlan) (receiptOutcome, error) {
	var out receiptOutcome
	var copied []string
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		replayed, ok, err := s.replayReceipt(ctx, by, k)
		if err != nil || ok {
			out = replayed
			return err
		}
		c, err := s.call(ctx, by)
		if err != nil {
			return err
		}
		if out, err = s.writeReceipt(ctx, c, k, plan); err != nil {
			return err
		}
		copied, err = s.promote(ctx, plan.staged)
		return err
	})
	if err != nil {
		for _, key := range copied {
			s.d.Photos.ReportOrphan(ctx, key)
		}
	}
	return out, err
}

// writeReceipt is saveReceipt's writes, but for the photos.
func (s *Service) writeReceipt(ctx context.Context, c call, k appshared.IdempotencyKey, plan receiptPlan) (receiptOutcome, error) {
	got, err := s.applyEntries(ctx, c, plan)
	if err != nil {
		return receiptOutcome{}, err
	}
	receipt, err := purchasing.NewReceipt(plan.spec(c))
	if err != nil {
		return receiptOutcome{}, err
	}
	if err := s.d.Repo.InsertReceipt(ctx, receipt); err != nil {
		return receiptOutcome{}, fmt.Errorf("save receipt %s: %w", receipt.ID(), err)
	}
	recs := make([]purchasing.LineRecord, len(got))
	orders := make([]shared.OrderID, len(got))
	for i, r := range got {
		recs[i], orders[i] = r.line.Record(), r.line.Item().Order
	}
	f, err := s.facts(ctx, orders...)
	if err != nil {
		return receiptOutcome{}, err
	}
	if err := s.moveReceived(ctx, got, f); err != nil {
		return receiptOutcome{}, err
	}
	if err := s.d.Idempotency.Remember(ctx, k, receipt.ID().String(), idempotencyTTL); err != nil {
		return receiptOutcome{}, fmt.Errorf("remember idempotency key: %w", err)
	}
	return receiptOutcome{view: receiptView(receipt.ID(), c.now, recs, f), orders: distinct(orders)}, nil
}

// promote copies every checked photo to its place and returns the keys that
// may have been copied, also when it fails.
func (s *Service) promote(ctx context.Context, photos []staged) ([]string, error) {
	var copied []string
	for _, st := range photos {
		err := s.d.Photos.Promote(ctx, st.photo.Key, st.etag, st.photo.ContentType)
		if mayHaveCopied(err) {
			copied = append(copied, st.photo.Key)
		}
		if err != nil {
			return copied, storageError("promote the upload", err)
		}
	}
	return copied, nil
}

// received is a line a receipt checked, with the pedido de compra its item
// moves under ("" at the counter).
type received struct {
	line purchasing.Line
	ref  string
}

// applyEntries checks every entry of the plan, in order: an open line of a
// supplier order is received on it, an item bought at the counter gets a
// line of its own. Each supplier order touched is saved once, before the
// receipt that points at its lines.
func (s *Service) applyEntries(ctx context.Context, c call, plan receiptPlan) ([]received, error) {
	w := entryWork{s: s, c: c, orders: map[shared.SupplierOrderID]*purchasing.SupplierOrder{}}
	out := make([]received, len(plan.entries))
	for i, e := range plan.entries {
		var err error
		if e.counter() {
			out[i], err = w.counterLine(ctx, e)
		} else {
			out[i], err = w.supplierLine(ctx, e)
		}
		if err != nil {
			return nil, err
		}
	}
	for _, id := range w.touched {
		so := w.orders[id]
		if err := s.d.Repo.UpdateSupplierOrder(ctx, so); err != nil {
			return nil, fmt.Errorf("save supplier order %s: %w", id, err)
		}
	}
	return out, nil
}

// entryWork is the supplier orders and the items to buy one receipt read
// once.
type entryWork struct {
	s       *Service
	c       call
	orders  map[shared.SupplierOrderID]*purchasing.SupplierOrder
	touched []shared.SupplierOrderID
	toBuy   map[shared.OrderItemID]ToBuyItem
}

// supplierLine receives an open line on its supplier order.
func (w *entryWork) supplierLine(ctx context.Context, e entryPlan) (received, error) {
	rec, err := w.s.d.Repo.SupplierOrderOfLine(ctx, w.c.by.TenantID(), e.line)
	if err != nil {
		return received{}, fmt.Errorf("find supplier order of line %s: %w", e.line, err)
	}
	so, ok := w.orders[rec.ID]
	if !ok {
		if so, err = restore(rec); err != nil {
			return received{}, err
		}
		w.orders[rec.ID] = so
		w.touched = append(w.touched, rec.ID)
	}
	l, err := so.Receive(e.line, e.in.Check, e.in.Note, w.c.today)
	return received{line: l, ref: so.Number().String()}, err
}

// counterLine writes the line of an item bought at the counter. The item
// must still be in "A comprar".
func (w *entryWork) counterLine(ctx context.Context, e entryPlan) (received, error) {
	if w.toBuy == nil {
		tb, err := w.s.d.Orders.ToBuy(ctx)
		if err != nil {
			return received{}, fmt.Errorf("read items to buy: %w", err)
		}
		w.toBuy = make(map[shared.OrderItemID]ToBuyItem, len(tb.Items))
		for _, it := range tb.Items {
			w.toBuy[it.Item.OrderItem] = it
		}
	}
	it, ok := w.toBuy[e.in.OrderItem]
	if !ok {
		return received{}, fmt.Errorf("item %s: %w", e.in.OrderItem, ErrItemNotToBuy)
	}
	// The repository copies the order-time cost in SQL, as for a sent line.
	l, err := purchasing.CounterLine(purchasing.CounterSpec{
		ID: e.line, Supplier: it.Supplier, Item: it.Item, Check: e.in.Check, Note: e.in.Note, Today: w.c.today,
	})
	if err != nil {
		return received{}, err
	}
	if err := w.s.d.Repo.InsertCounterLine(ctx, w.c.by.TenantID(), l); err != nil {
		return received{}, fmt.Errorf("save counter line %s: %w", l.ID(), err)
	}
	return received{line: l}, nil
}

// moveReceived moves the items of the received lines: "received" with the
// check, which lands a defect on "A comprar". Items of a cancelled order
// or of a removed item stay where they are (see orderFacts.movable).
func (s *Service) moveReceived(ctx context.Context, got []received, f orderFacts) error {
	var moves []ItemMove
	for _, r := range got {
		it := r.line.Item()
		if !f.movable(it.Order, it.OrderItem) {
			continue
		}
		check := checkOf(r.line.Status())
		moves = append(moves, ItemMove{
			Order: it.Order, Item: it.OrderItem, To: order.ItemReceived, Check: &check, Note: r.line.CheckNote(), Ref: r.ref,
		})
	}
	if len(moves) == 0 {
		return nil
	}
	if err := s.d.Orders.Move(ctx, moves); err != nil {
		return fmt.Errorf("move received items: %w", err)
	}
	return nil
}

// checkOf is the check a closed line records.
func checkOf(st purchasing.LineStatus) purchasing.Check {
	if st == purchasing.LineDefective {
		return purchasing.CheckDefective
	}
	return purchasing.CheckOK
}

// receiptView is a receipt of lines, in the order of its entries. A defect
// whose item could be moved is "back to buy".
func receiptView(id shared.ReceiptID, at time.Time, lines []purchasing.LineRecord, f orderFacts) ReceiptView {
	v := ReceiptView{ID: id, ReceivedAt: at}
	for _, l := range lines {
		it := l.Item
		v.Entries = append(v.Entries, ReceiptEntryView{
			Line: l.ID, Order: it.Order, OrderNumber: f[it.Order].Number, OrderItem: it.OrderItem,
			Name: it.Name, Check: checkOf(l.Status),
		})
		if l.Status == purchasing.LineDefective && f.movable(it.Order, it.OrderItem) {
			v.BackToBuy = append(v.BackToBuy, BackToBuyView{OrderItem: it.OrderItem, Name: it.Name})
		}
	}
	return v
}

// completed adds the orders whose every item has now arrived, read after
// the transaction.
func (s *Service) completed(ctx context.Context, out receiptOutcome) (ReceiptView, error) {
	f, err := s.facts(ctx, out.orders...)
	if err != nil {
		return ReceiptView{}, err
	}
	v := out.view
	for _, id := range out.orders {
		o := f[id]
		if !o.Closed && len(o.Items) > 0 && !slices.ContainsFunc(o.Items, func(it OrderItemFacts) bool { return !it.Arrived }) {
			v.CompletedOrders = append(v.CompletedOrders, OrderRefView{Order: id, Number: o.Number})
		}
	}
	return v, nil
}

// distinct returns ids without repeats, in order of first appearance.
func distinct(ids []shared.OrderID) []shared.OrderID {
	var out []shared.OrderID
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
