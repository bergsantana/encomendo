package catalogports

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Harness is the catalog repositories with what the suite needs to use
// them.
type Harness struct {
	Categories appcatalog.Categories
	Attributes appcatalog.Attributes
	Items      appcatalog.Items
	Kits       appcatalog.Kits
	Tx         appshared.TxManager
	NewID      func() shared.ID
	// NewTenant returns a store rows may belong to. The Postgres harness
	// seeds a tenants row; the fake needs none.
	NewTenant func(t *testing.T) shared.TenantID
	// NewSupplier returns a supplier of tenant that items may link to. The
	// Postgres harness seeds a suppliers row; the fake needs none.
	NewSupplier func(t *testing.T, tenant shared.TenantID) shared.SupplierID
}

// tenant is one store of the suite with the staff user that acts in it.
type tenant struct {
	id   shared.TenantID
	user shared.UserID
}

func (h Harness) newTenant(t *testing.T) tenant {
	t.Helper()
	return tenant{id: h.NewTenant(t), user: shared.UserID(h.NewID())}
}

func (tn tenant) scope() appshared.Scope { return appshared.Scope{TenantID: tn.id, UserID: tn.user} }

// storePage is the store page's scope: the tenant and no user.
func (tn tenant) storePage() appshared.Scope { return appshared.Scope{TenantID: tn.id} }

func (h Harness) in(t *testing.T, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s, fn); err != nil {
		t.Fatal(err)
	}
}

func (h Harness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

var errRollback = errors.New("catalogports: the transaction fails on purpose")

func brl(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func page(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// itemSpec describes an item the suite builds.
type itemSpec struct {
	name string
	// supplier, when set, is the main supplier instead of a new one of the
	// item's store.
	supplier  shared.SupplierID
	category  shared.CategoryID
	inHouse   bool
	values    []shared.AttributeValueID
	published bool
	archived  bool
}

// buildItem returns a valid item of tn: to_order from a new supplier at 7
// days and R$ 40 with a backup at 9 days without a cost, or in_house at 5
// days and R$ 12. It is not stored.
func (h Harness) buildItem(t *testing.T, tn tenant, spec itemSpec) *catalog.Item {
	t.Helper()
	src := catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 5, ProductionCost: catalog.SetCost(brl(t, 1200))}
	if !spec.inHouse {
		main := spec.supplier
		if main.IsZero() {
			main = h.NewSupplier(t, tn.id)
		}
		src = catalog.SourcingInput{
			Mode:   catalog.ModeToOrder,
			Main:   &catalog.SupplierLink{Supplier: main, LeadDays: 7, Cost: catalog.SetCost(brl(t, 4000))},
			Backup: &catalog.SupplierLink{Supplier: h.NewSupplier(t, tn.id), LeadDays: 9, Cost: catalog.ClearCost()},
		}
	}
	choice := catalog.ChoiceInput{ID: shared.ChoiceID(h.NewID()), Label: "Vermelha", ExtraPrice: brl(t, 500), ExtraDays: 2}
	it, err := catalog.NewItem(tn.id, shared.ItemID(h.NewID()), catalog.ItemInput{
		ItemDetails: catalog.ItemDetails{
			Name: spec.name, Description: "Couro legítimo", Category: spec.category, Price: brl(t, 10000),
			AttributeValues: spec.values,
			Options:         []catalog.OptionInput{{ID: shared.OptionID(h.NewID()), Name: "Cor da costura", Required: true, Choices: []catalog.ChoiceInput{choice}}},
		},
		Sourcing: src,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.published {
		if err := it.Publish(); err != nil {
			t.Fatal(err)
		}
	}
	if spec.archived {
		it.Archive()
	}
	return it
}

// addItem builds and stores an item of tn.
func (h Harness) addItem(t *testing.T, tn tenant, spec itemSpec) *catalog.Item {
	t.Helper()
	it := h.buildItem(t, tn, spec)
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Items.Insert(ctx, it) })
	return it
}

// kitSpec describes a kit the suite builds.
type kitSpec struct {
	name      string
	items     []catalog.KitItem
	values    []shared.AttributeValueID
	published bool
	archived  bool
}

func (h Harness) buildKit(t *testing.T, tn tenant, spec kitSpec) *catalog.Kit {
	t.Helper()
	lookup := catalog.ItemLookup{}
	for _, ki := range spec.items {
		for _, id := range append([]shared.ItemID{ki.Item}, ki.Alternatives...) {
			lookup[id] = catalog.ItemFacts{Price: brl(t, 10000)}
		}
	}
	assembly := 3
	k, err := catalog.NewKit(tn.id, shared.KitID(h.NewID()), catalog.KitInput{
		Name: spec.name, Price: brl(t, 15000), SwapMode: catalog.SwapLoseDiscount, AssemblyDays: &assembly,
		Items: spec.items, AttributeValues: spec.values,
	}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if spec.published {
		if err := k.Publish(); err != nil {
			t.Fatal(err)
		}
	}
	if spec.archived {
		k.Archive()
	}
	return k
}

func (h Harness) addKit(t *testing.T, tn tenant, spec kitSpec) *catalog.Kit {
	t.Helper()
	k := h.buildKit(t, tn, spec)
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Kits.Insert(ctx, k) })
	return k
}

func (h Harness) addCategory(t *testing.T, tn tenant, name string, position int) *catalog.Category {
	t.Helper()
	c, err := catalog.NewCategory(tn.id, shared.CategoryID(h.NewID()), name, position)
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Categories.Insert(ctx, c) })
	return c
}

func (h Harness) addAttribute(t *testing.T, tn tenant, name string, position int, labels ...string) *catalog.Attribute {
	t.Helper()
	values := make([]catalog.ValueInput, 0, len(labels))
	for _, l := range labels {
		values = append(values, catalog.ValueInput{ID: shared.AttributeValueID(h.NewID()), Label: l})
	}
	a, err := catalog.NewAttribute(tn.id, shared.AttributeID(h.NewID()), catalog.AttributeInput{Name: name, Values: values})
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Attributes.Insert(ctx, a, position) })
	return a
}

func valueIDs(a *catalog.Attribute) []shared.AttributeValueID {
	var out []shared.AttributeValueID
	for _, v := range a.Values() {
		out = append(out, v.ID())
	}
	return out
}

func member(id shared.ItemID) catalog.KitItem { return catalog.KitItem{Item: id, Quantity: 1} }

// isCursorError reports whether err is a ValidationError on "cursor".
func isCursorError(err error) bool {
	var ve *shared.ValidationError
	return errors.As(err, &ve) && len(ve.Fields) == 1 && ve.Fields[0].Field == "cursor"
}
