package catalog_test

import (
	"errors"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// banco is suppliedInput's item with its required option's two choices:
// Vermelha (+R$ 5,00, +2 days) and Preta.
func (h *harness) banco(t *testing.T) (item appcatalog.ItemView, red, black appcatalog.OptionChoice) {
	t.Helper()
	item = h.createItem(t, h.suppliedInput("Banco"))
	opt := item.Options[0]
	red = appcatalog.OptionChoice{Item: item.ID, Option: opt.ID, Choice: opt.Choices[0].ID}
	black = appcatalog.OptionChoice{Item: item.ID, Option: opt.ID, Choice: opt.Choices[1].ID}
	return item, red, black
}

func TestOrderLines_LooseItemWithOptionAndSourcing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	item, red, _ := h.banco(t)

	lines, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{{Item: item.ID, Quantity: 3, Options: []appcatalog.OptionChoice{red}}})
	if err != nil {
		t.Fatal(err)
	}
	l := lines[0]
	// (100,00 + 5,00) × 3
	if l.Kit != nil || l.Price.Cents() != 31_500 || len(l.Items) != 1 {
		t.Fatalf("line = %+v, want one loose item at R$ 315,00", l)
	}
	it := l.Items[0]
	if it.Quantity != 3 || it.ReadyDays != 7 || len(it.Options) != 1 || it.Options[0].ExtraDays != 2 || it.Options[0].Label != "Vermelha" {
		t.Errorf("item = %+v", it)
	}
	if it.MainSupplier == nil || it.MainSupplier.Name != "Couros Paulista" || it.MainSupplier.LeadDays != 7 {
		t.Errorf("main supplier = %+v", it.MainSupplier)
	}
	if it.BackupSupplier == nil || it.BackupSupplier.ID != h.ferragens || it.BackupSupplier.LeadDays != 9 {
		t.Errorf("backup supplier = %+v", it.BackupSupplier)
	}
}

func TestOrderLines_ReadsCostsForAnyOrderWriter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := inHouseInput("Assento", 20_000, 4)
	in.ProductionCost = appcatalog.SetCost(8_000)
	item := h.createItem(t, in)

	// The order snapshot keeps the cost for the owner's margins, whoever
	// takes the order; the order views hide it from other roles.
	lines, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{{Item: item.ID, Quantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if c := lines[0].Items[0].Cost; c == nil || c.Cents() != 8_000 || lines[0].Items[0].MainSupplier != nil {
		t.Errorf("in-house item = %+v, want its production cost and no supplier", lines[0].Items[0])
	}
}

func TestOrderLines_KitWithSwapKeepsDiscountAndAddsOptionExtras(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	banco, red, _ := h.banco(t)
	simple := h.createItem(t, inHouseInput("Banco simples", 6_000, 3))
	alt := h.createItem(t, inHouseInput("Banco alto", 15_000, 10))
	guidao := h.createItem(t, inHouseInput("Guidão", 8_000, 5))
	in := kitOf("Kit Scrambler", 20_000,
		catalog.KitItem{Item: simple.ID, Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{alt.ID}},
		catalog.KitItem{Item: banco.ID, Quantity: 2},
		one(guidao.ID))
	in.AssemblyDays = ptr(4)
	kit := h.createKit(t, in)

	lines, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{{
		Kit: kit.ID, Swaps: []catalog.Swap{{Item: simple.ID, With: alt.ID}}, Options: []appcatalog.OptionChoice{red},
	}})
	if err != nil {
		t.Fatal(err)
	}
	l := lines[0]
	// keep: 200,00 − 60,00 + 150,00 = 290,00; the banco's option × 2 = +10,00.
	if l.Price.Cents() != 30_000 {
		t.Errorf("price = %v, want R$ 300,00", l.Price)
	}
	if l.Kit == nil || len(l.Kit.Swaps) != 1 || l.Kit.Swaps[0].AddedName != "Banco alto" || l.Kit.Swaps[0].RemovedName != "Banco simples" {
		t.Errorf("kit = %+v", l.Kit)
	}
	if len(l.Items) != 3 || l.Items[0].Item != alt.ID || l.Items[1].Quantity != 2 {
		t.Errorf("items = %+v, want the swapped item first and the banco twice", l.Items)
	}
	if l.AssemblyDays == nil || *l.AssemblyDays != 4 || l.SafetyMarginDays != nil {
		t.Errorf("kit days = %v %v", l.AssemblyDays, l.SafetyMarginDays)
	}
}

func TestOrderLines_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	banco, red, black := h.banco(t)
	plain := h.createItem(t, inHouseInput("Simples", 2_000, 1))
	archived := h.createItem(t, inHouseInput("Antigo", 1_000, 1))
	if _, err := h.svc.ArchiveItem(h.owner(t), appcatalog.ItemVersionInput{ID: archived.ID, Version: archived.Version}); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		req  appcatalog.OrderLineRequest
		want error
	}{
		"required option missing": {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 1}, shared.ErrInvalid},
		"two choices":             {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 1, Options: []appcatalog.OptionChoice{red, black}}, shared.ErrInvalid},
		"choice of another option": {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 1, Options: []appcatalog.OptionChoice{
			{Item: banco.ID, Option: red.Option, Choice: shared.ChoiceID(h.ids.NewID())}}}, shared.ErrInvalid},
		"choice for another item": {appcatalog.OrderLineRequest{Item: plain.ID, Quantity: 1, Options: []appcatalog.OptionChoice{red}}, shared.ErrInvalid},
		"stale option id": {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 1, Options: []appcatalog.OptionChoice{red,
			{Item: banco.ID, Option: shared.OptionID(h.ids.NewID()), Choice: red.Choice}}}, shared.ErrInvalid},
		"swaps on a loose item": {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 1, Options: []appcatalog.OptionChoice{red},
			Swaps: []catalog.Swap{{Item: banco.ID, With: plain.ID}}}, shared.ErrInvalid},
		"quantity 0":    {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 0, Options: []appcatalog.OptionChoice{red}}, shared.ErrInvalid},
		"quantity 100":  {appcatalog.OrderLineRequest{Item: banco.ID, Quantity: 100, Options: []appcatalog.OptionChoice{red}}, shared.ErrInvalid},
		"kit and item":  {appcatalog.OrderLineRequest{Item: banco.ID, Kit: shared.KitID(h.ids.NewID()), Quantity: 1}, shared.ErrInvalid},
		"neither":       {appcatalog.OrderLineRequest{Quantity: 1}, shared.ErrInvalid},
		"archived item": {appcatalog.OrderLineRequest{Item: archived.ID, Quantity: 1}, catalog.ErrItemArchived},
		"unknown item":  {appcatalog.OrderLineRequest{Item: shared.ItemID(h.ids.NewID()), Quantity: 1}, catalog.ErrItemNotFound},
		"unknown kit":   {appcatalog.OrderLineRequest{Kit: shared.KitID(h.ids.NewID())}, catalog.ErrKitNotFound},
	}
	for name, tt := range tests {
		if _, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{tt.req}); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tt.want)
		}
	}
}

func TestOrderLines_ArchivedKitAndOptionForAnItemOutsideTheKit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, red, _ := h.banco(t)
	guidao := h.createItem(t, inHouseInput("Guidão", 8_000, 5))
	kit := h.createKit(t, kitOf("Kit", 7_000, one(guidao.ID)))

	if _, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{{Kit: kit.ID, Options: []appcatalog.OptionChoice{red}}}); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("option of an item outside the kit: err = %v, want ErrInvalid", err)
	}
	if _, err := h.svc.ArchiveKit(h.owner(t), appcatalog.KitVersionInput{ID: kit.ID, Version: kit.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.OrderLines(h.staff(t), []appcatalog.OrderLineRequest{{Kit: kit.ID}}); !errors.Is(err, catalog.ErrItemArchived) {
		t.Errorf("archived kit: err = %v, want ErrItemArchived", err)
	}
}

func TestOrderLines_NeedsOrderWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	item := h.createItem(t, inHouseInput("Assento", 20_000, 4))
	if _, err := h.svc.OrderLines(h.viewer(t), []appcatalog.OrderLineRequest{{Item: item.ID, Quantity: 1}}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v, want ErrForbidden", err)
	}
}
