// Package fake holds in-memory versions of app/order's driven ports, for
// use case tests. Like adapter/postgres, the repository refuses calls
// outside a fake transaction and a failed transaction undoes its writes.
package fake

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sync"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

type orderKey struct {
	tenant shared.TenantID
	id     shared.OrderID
}

// Orders is an in-memory apporder.Repository. Rows are visible only in a
// scope of their store, as tenant_isolation shows them.
type Orders struct {
	mu       sync.Mutex
	rows     map[orderKey]order.Record
	numbers  map[shared.TenantID]order.Number
	history  map[orderKey][]order.HistoryEntry
	messages []apporder.Message
	fail     error

	workflows apporder.Workflows // set by Link
	customers apporder.Customers
	userNames map[shared.UserID]string
}

var _ apporder.Repository = (*Orders)(nil)

// NewOrders returns an empty repository.
func NewOrders() *Orders {
	return &Orders{rows: map[orderKey]order.Record{}, numbers: map[shared.TenantID]order.Number{}, history: map[orderKey][]order.HistoryEntry{}}
}

// Fail makes every later call return err; nil heals it.
func (r *Orders) Fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail = err
}

func (r *Orders) enter(ctx context.Context, tenant shared.TenantID) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	r.mu.Lock()
	if r.fail != nil {
		r.mu.Unlock()
		return r.fail
	}
	if appshared.ScopeFrom(ctx).TenantID != tenant {
		r.mu.Unlock()
		return errors.New("fake: new row violates row-level security policy")
	}
	return nil
}

// NextNumber implements apporder.Repository.
func (r *Orders) NextNumber(ctx context.Context, tenant shared.TenantID) (order.Number, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return 0, err
	}
	defer r.mu.Unlock()
	sharedfake.KeepOnRollback(ctx, &r.mu, r.numbers, tenant)
	r.numbers[tenant]++
	return r.numbers[tenant], nil
}

// Insert implements apporder.Repository.
func (r *Orders) Insert(ctx context.Context, o *order.Order) error {
	if err := r.enter(ctx, o.TenantID()); err != nil {
		return err
	}
	defer r.mu.Unlock()
	k := orderKey{o.TenantID(), o.ID()}
	if _, ok := r.rows[k]; ok {
		return fmt.Errorf("fake: order %s exists: %w", o.ID(), shared.ErrConflict)
	}
	rec := o.Record()
	if err := r.heldElsewhere(k, rec); err != nil {
		return err
	}
	r.store(ctx, k, rec, o.PendingHistory())
	return nil
}

// heldElsewhere refuses a line or item id another order holds, of any
// store, as Postgres's primary keys and its ON CONFLICT guard do. The caller
// holds the lock.
func (r *Orders) heldElsewhere(k orderKey, rec order.Record) error {
	mine := map[[16]byte]bool{}
	for _, l := range rec.Lines {
		mine[l.ID] = true
		for _, it := range l.Items {
			mine[it.ID] = true
		}
	}
	for other, o := range r.rows {
		if other == k {
			continue
		}
		for _, l := range o.Lines {
			if mine[l.ID] {
				return fmt.Errorf("fake: line %s belongs to another order: %w", l.ID, shared.ErrInvalid)
			}
			for _, it := range l.Items {
				if mine[it.ID] {
					return fmt.Errorf("fake: item %s belongs to another order: %w", it.ID, shared.ErrInvalid)
				}
			}
		}
	}
	return nil
}

// Update implements apporder.Repository: the stored version must still be
// o's, and the next one is stored.
func (r *Orders) Update(ctx context.Context, o *order.Order) error {
	if err := r.enter(ctx, o.TenantID()); err != nil {
		return err
	}
	defer r.mu.Unlock()
	k := orderKey{o.TenantID(), o.ID()}
	stored, ok := r.rows[k]
	if !ok {
		return apporder.ErrNotFound
	}
	if stored.Version != o.Version() {
		return shared.ErrVersionConflict
	}
	rec := o.Record()
	if err := r.heldElsewhere(k, rec); err != nil {
		return err
	}
	rec.Version++
	r.store(ctx, k, rec, o.PendingHistory())
	return nil
}

func (r *Orders) store(ctx context.Context, k orderKey, rec order.Record, pending []order.HistoryEntry) {
	sharedfake.KeepOnRollback(ctx, &r.mu, r.rows, k)
	sharedfake.KeepOnRollback(ctx, &r.mu, r.history, k)
	r.rows[k] = rec
	r.history[k] = append(slices.Clone(r.history[k]), pending...)
}

// ByID implements apporder.Repository.
func (r *Orders) ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (order.Record, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return order.Record{}, err
	}
	defer r.mu.Unlock()
	rec, ok := r.rows[orderKey{tenant, id}]
	if !ok {
		return order.Record{}, apporder.ErrNotFound
	}
	return rec, nil
}

// AddMessage implements apporder.Repository.
func (r *Orders) AddMessage(ctx context.Context, m apporder.Message) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	before := len(r.messages)
	sharedfake.OnRollback(ctx, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.messages = r.messages[:before]
	})
	r.messages = append(r.messages, m)
	return nil
}

// Stored returns the committed record of an order, for assertions.
func (r *Orders) Stored(tenant shared.TenantID, id shared.OrderID) (order.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.rows[orderKey{tenant, id}]
	return rec, ok
}

// Written returns the history written for an order, oldest first, for
// assertions; History is the repository query.
func (r *Orders) Written(tenant shared.TenantID, id shared.OrderID) []order.HistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.history[orderKey{tenant, id}])
}

// Messages returns every recorded message, oldest first.
func (r *Orders) Messages() []apporder.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.messages)
}

// CatalogItem is an item the fake catalog sells.
type CatalogItem struct {
	ID       shared.ItemID
	Name     string
	Mode     catalog.SourcingMode
	Price    shared.Money
	LeadDays int
	Sourcing order.Sourcing
	Options  []order.ChosenOption // the choices clients may pick
	Archived bool
}

// CatalogKit is a kit the fake catalog sells at a fixed price; swaps are not
// modelled (app/catalog's tests cover them).
type CatalogKit struct {
	ID       shared.KitID
	Name     string
	Price    shared.Money
	Items    map[shared.ItemID]int // item → quantity
	Order    []shared.ItemID
	Assembly *int
}

// Catalog is an apporder.Catalog over fixtures.
type Catalog struct {
	mu    sync.Mutex
	items map[shared.ItemID]CatalogItem
	kits  map[shared.KitID]CatalogKit
}

var _ apporder.Catalog = (*Catalog)(nil)

// NewCatalog returns an empty catalog.
func NewCatalog() *Catalog {
	return &Catalog{items: map[shared.ItemID]CatalogItem{}, kits: map[shared.KitID]CatalogKit{}}
}

// AddItem adds or replaces an item.
func (c *Catalog) AddItem(it CatalogItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[it.ID] = it
}

// AddKit adds or replaces a kit.
func (c *Catalog) AddKit(k CatalogKit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kits[k.ID] = k
}

// OrderLines implements apporder.Catalog: (price + chosen extras) ×
// quantity for a loose item, the kit price plus extras for a kit.
func (c *Catalog) OrderLines(_ context.Context, reqs []apporder.LineRequest) ([]order.LineSpec, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]order.LineSpec, 0, len(reqs))
	for _, r := range reqs {
		l, err := c.line(r)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func (c *Catalog) line(r apporder.LineRequest) (order.LineSpec, error) {
	if r.Kit.IsZero() {
		it, err := c.item(r.Item, r.Quantity, r.Options)
		if err != nil {
			return order.LineSpec{}, err
		}
		price, err := catalog.PriceWithOptions(it.Snapshot.UnitPrice, extras(it.Snapshot.Options), r.Quantity)
		return order.LineSpec{Price: price, Items: []order.ItemSpec{it}}, err
	}
	k, ok := c.kits[r.Kit]
	if !ok {
		return order.LineSpec{}, catalog.ErrKitNotFound
	}
	l := order.LineSpec{Kit: &order.KitSnapshot{Kit: k.ID, Name: k.Name}, Price: k.Price, AssemblyDays: k.Assembly}
	for _, id := range k.Order {
		it, err := c.item(id, k.Items[id], r.Options)
		if err != nil {
			return order.LineSpec{}, err
		}
		if l.Price, err = catalog.AddOptionExtras(l.Price, extras(it.Snapshot.Options), k.Items[id]); err != nil {
			return order.LineSpec{}, err
		}
		l.Items = append(l.Items, it)
	}
	return l, nil
}

func extras(options []order.ChosenOption) []shared.Money {
	out := make([]shared.Money, len(options))
	for i, o := range options {
		out[i] = o.ExtraPrice
	}
	return out
}

func (c *Catalog) item(id shared.ItemID, qty int, choices []apporder.OptionChoice) (order.ItemSpec, error) {
	it, ok := c.items[id]
	switch {
	case !ok:
		return order.ItemSpec{}, catalog.ErrItemNotFound
	case it.Archived:
		return order.ItemSpec{}, &catalog.ArchivedItemsError{Items: []shared.ItemID{id}}
	}
	snap := order.ItemSnapshot{CatalogItem: id, Name: it.Name, SourcingMode: it.Mode, UnitPrice: it.Price, Quantity: qty, LeadDays: it.LeadDays}
	for _, ch := range choices {
		for _, o := range it.Options {
			if ch.Item == id && ch.Option == o.Option && ch.Choice == o.Choice {
				snap.Options = append(snap.Options, o)
			}
		}
	}
	return order.ItemSpec{Snapshot: snap, Sourcing: it.Sourcing}, nil
}

// Customers is an apporder.Customers in memory, for one store.
type Customers struct {
	mu   sync.Mutex
	byID map[shared.CustomerID]apporder.CustomerFacts
	next func() shared.ID
}

var _ apporder.Customers = (*Customers)(nil)

// NewCustomers returns a store with no customers; next makes new ids.
func NewCustomers(next func() shared.ID) *Customers {
	return &Customers{byID: map[shared.CustomerID]apporder.CustomerFacts{}, next: next}
}

// Add stores c.
func (f *Customers) Add(c apporder.CustomerFacts) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[c.ID] = c
}

// Get implements apporder.Customers.
func (f *Customers) Get(_ context.Context, id shared.CustomerID) (apporder.CustomerFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byID[id]
	if !ok {
		return apporder.CustomerFacts{}, fmt.Errorf("fake: customer %s: %w", id, shared.ErrNotFound)
	}
	return c, nil
}

// FindOrCreate implements apporder.Customers. A customer it creates goes
// away when the caller's transaction fails, as in Postgres.
func (f *Customers) FindOrCreate(ctx context.Context, name, phone string) (apporder.CustomerFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.byID {
		if c.Phone == phone {
			return c, nil
		}
	}
	c := apporder.CustomerFacts{ID: shared.CustomerID(f.next()), Name: name, Phone: phone}
	sharedfake.KeepOnRollback(ctx, &f.mu, f.byID, c.ID)
	f.byID[c.ID] = c
	return c, nil
}

// Count returns how many customers the store has.
func (f *Customers) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byID)
}

// Workflows is an apporder.Workflows: a current workflow and every version
// it was given.
type Workflows struct {
	mu       sync.Mutex
	current  workflow.Workflow
	versions []workflow.Workflow
}

var _ apporder.Workflows = (*Workflows)(nil)

// NewWorkflows returns a source whose current workflow is wf.
func NewWorkflows(wf workflow.Workflow) *Workflows {
	return &Workflows{current: wf, versions: []workflow.Workflow{wf}}
}

// Choose makes wf the current workflow, as the owner's choice in settings.
func (f *Workflows) Choose(wf workflow.Workflow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = wf
	f.versions = append(f.versions, wf)
}

// Current implements apporder.Workflows.
func (f *Workflows) Current(context.Context) (workflow.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current, nil
}

// Get implements apporder.Workflows.
func (f *Workflows) Get(_ context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.versions {
		if w.ID() == id && w.Version() == version {
			return w, nil
		}
	}
	return workflow.Workflow{}, fmt.Errorf("fake: workflow %s v%d: %w", id, version, shared.ErrNotFound)
}

// Store is an apporder.Store with fixed facts.
type Store struct{ facts apporder.StoreFacts }

// NewStore returns a store whose facts are f.
func NewStore(f apporder.StoreFacts) Store { return Store{facts: f} }

// Facts implements apporder.Store.
func (s Store) Facts(context.Context) (apporder.StoreFacts, error) { return s.facts, nil }

// Tracking builds tracking URLs on a test domain.
type Tracking struct{}

// URL implements apporder.TrackingURLs.
func (Tracking) URL(slug string, token shared.Secret) (string, error) {
	return "https://" + slug + ".encomendo.test/p/" + token.Reveal(), nil
}

// WhatsApp builds wa.me links.
type WhatsApp struct{}

// Link implements apporder.WhatsApp.
func (WhatsApp) Link(phone, text string) (string, error) {
	return "https://wa.me/" + phone + "?text=" + url.QueryEscape(text), nil
}
