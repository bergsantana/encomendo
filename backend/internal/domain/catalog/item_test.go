package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

func TestNewItem_ToOrder(t *testing.T) {
	t.Parallel()

	in := toOrderInput(t)
	in.Name = "  Banco de couro Scrambler  "
	in.Description = "  Couro legítimo.\nCostura vermelha.  "
	in.Category = categoryID(9)
	in.AttributeValues = []shared.AttributeValueID{attrValueID(1), attrValueID(2)}
	in.Sourcing.Backup = link(2, 20, SetCost(moneyOf(t, 27000)))
	i := newItem(t, 1, in)

	if i.TenantID() != tenantID || i.ID() != itemID(1) || i.Version() != 1 {
		t.Errorf("identity = %v %v v%d", i.TenantID(), i.ID(), i.Version())
	}
	if i.Name() != "Banco de couro Scrambler" || i.Description() != "Couro legítimo.\nCostura vermelha." {
		t.Errorf("name %q description %q, want both trimmed", i.Name(), i.Description())
	}
	if i.CategoryID() != categoryID(9) || i.Price().Cents() != 40000 || i.SourcingMode() != ModeToOrder {
		t.Errorf("category %v price %v mode %q", i.CategoryID(), i.Price(), i.SourcingMode())
	}
	main, ok := i.MainSupplier()
	if !ok || main.SupplierID() != supplierID(1) || main.Role() != supplier.RoleMain || main.LeadDays() != 15 {
		t.Errorf("main = %+v, %v", main, ok)
	}
	backup, ok := i.BackupSupplier()
	if !ok || backup.SupplierID() != supplierID(2) || backup.Role() != supplier.RoleBackup {
		t.Errorf("backup = %+v, %v", backup, ok)
	}
	if i.ProductionDays() != 0 || i.ProductionCost() != nil {
		t.Errorf("production days %d cost %v, want none", i.ProductionDays(), i.ProductionCost())
	}
	if i.Published() || i.Archived() {
		t.Error("a new item must be unpublished and not archived")
	}
	if got := i.AttributeValueIDs(); len(got) != 2 || got[0] != attrValueID(1) {
		t.Errorf("attribute values = %v", got)
	}
}

func TestNewItem_StockWithoutBackup(t *testing.T) {
	t.Parallel()

	in := toOrderInput(t)
	in.Sourcing.Mode = ModeStock
	in.Sourcing.Main = link(1, 4, KeepCost())
	i := newItem(t, 1, in)

	if _, ok := i.BackupSupplier(); ok {
		t.Error("BackupSupplier ok = true, want no backup")
	}
	if i.Cost() != nil {
		t.Errorf("Cost = %v, want none (keep with nothing stored)", i.Cost())
	}
	if _, ok := i.Margin(); ok {
		t.Error("Margin ok = true, want none without a cost")
	}
}

func TestNewItem_InHouse(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, inHouseInput(t))

	if i.SourcingMode() != ModeInHouse || i.ProductionDays() != 3 {
		t.Errorf("mode %q days %d", i.SourcingMode(), i.ProductionDays())
	}
	if _, ok := i.MainSupplier(); ok {
		t.Error("an in_house item has no supplier")
	}
	if pc := i.ProductionCost(); pc == nil || pc.Cents() != 6000 {
		t.Errorf("ProductionCost = %v, want 6000", pc)
	}
}

func TestItem_ReadyDays(t *testing.T) {
	t.Parallel()

	inHouse := inHouseInput(t)
	inHouse.Sourcing.ProductionDays = 7
	stock := toOrderInput(t)
	stock.Sourcing.Mode = ModeStock
	stock.Sourcing.Main = link(1, 2, KeepCost())
	tests := []struct {
		name string
		in   ItemInput
		want int
	}{
		{"to_order is the main supplier's lead time", toOrderInput(t), 15},
		{"stock is the main supplier's lead time", stock, 2},
		{"in_house is the production days", inHouse, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			i := newItem(t, 1, tt.in)
			if got := i.ReadyDays(); got != tt.want {
				t.Errorf("ReadyDays = %d, want %d", got, tt.want)
			}
			if f := i.Facts(); f.ReadyDays != tt.want || f.Archived || f.Price != i.Price() {
				t.Errorf("Facts = %+v", f)
			}
		})
	}
}

func TestItem_BackupLeadTimeDoesNotChangeReadyDays(t *testing.T) {
	t.Parallel()

	in := toOrderInput(t)
	in.Sourcing.Backup = link(2, 90, KeepCost())
	if got := newItem(t, 1, in).ReadyDays(); got != 15 {
		t.Errorf("ReadyDays = %d, want the main supplier's 15", got)
	}
}

func TestItem_CostAndMargin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		price   int64
		cost    int64
		noCost  bool
		inHouse bool
		want    Margin
		wantOK  bool
	}{
		{"supplier cost 250 on price 400", 40000, 25000, false, false, Margin{15000, 3750}, true},
		{"in-house production cost", 15000, 6000, false, true, Margin{9000, 6000}, true},
		{"rounds toward zero", 30000, 20000 + 1, false, false, Margin{9999, 3333}, true},
		{"negative when the cost is above the price", 10000, 12500, false, false, Margin{-2500, -2500}, true},
		{"negative rounds toward zero too", 30000, 40000 + 1, false, false, Margin{-10001, -3333}, true},
		{"free item costs nothing", 10000, 0, false, false, Margin{10000, 10000}, true},
		{"no cost, no margin", 10000, 0, true, false, Margin{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := toOrderInput(t)
			if tt.inHouse {
				in = inHouseInput(t)
			}
			in.Price = moneyOf(t, tt.price)
			cost := SetCost(moneyOf(t, tt.cost))
			if tt.noCost {
				cost = ClearCost()
			}
			if tt.inHouse {
				in.Sourcing.ProductionCost = cost
			} else {
				in.Sourcing.Main.Cost = cost
			}
			i := newItem(t, 1, in)

			got, ok := i.Margin()
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("Margin = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
			if c := i.Cost(); (c == nil) != tt.noCost || (c != nil && c.Cents() != tt.cost) {
				t.Errorf("Cost = %v, want %d (none: %v)", c, tt.cost, tt.noCost)
			}
		})
	}
}

func TestItem_CostIsACopy(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	*i.Cost() = moneyOf(t, 1)
	if i.Cost().Cents() != 25000 {
		t.Error("changing the returned cost changed the item")
	}
	h := newItem(t, 2, inHouseInput(t))
	*h.ProductionCost() = moneyOf(t, 1)
	if h.ProductionCost().Cents() != 6000 {
		t.Error("changing the returned production cost changed the item")
	}
}

func TestMarginOf_Refusals(t *testing.T) {
	t.Parallel()

	if _, ok := MarginOf(moneyOf(t, 100), mustMoney(t, 50, "USD")); ok {
		t.Error("MarginOf across currencies must report false")
	}
	if _, ok := MarginOf(moneyOf(t, 0), moneyOf(t, 50)); ok {
		t.Error("MarginOf with a zero price must report false")
	}
}

func TestNewItem_Validation(t *testing.T) {
	t.Parallel()

	long := repeat("a", 121)
	tests := []struct {
		name   string
		mutate func(in *ItemInput)
		want   map[string]string
	}{
		{"blank name", func(in *ItemInput) { in.Name = "   " }, map[string]string{"name": msgItemNameRequired}},
		{"name over 120", func(in *ItemInput) { in.Name = long }, map[string]string{"name": msgNameTooLong}},
		{"name with a line break", func(in *ItemInput) { in.Name = "a\nb" }, map[string]string{"name": msgNameControl}},
		{"description over 2000", func(in *ItemInput) { in.Description = repeat("é", 2001) }, map[string]string{"description": msgDescriptionTooLong}},
		{"zero price", func(in *ItemInput) { in.Price = moneyOf(t, 0) }, map[string]string{"priceCents": msgItemPrice}},
		{"negative price", func(in *ItemInput) { in.Price = moneyOf(t, -5) }, map[string]string{"priceCents": msgItemPrice}},
		{"price too high", func(in *ItemInput) { in.Price = moneyOf(t, maxAmountCents+1) }, map[string]string{"priceCents": msgAmountTooHigh}},
		{"unknown mode", func(in *ItemInput) { in.Sourcing.Mode = "dropship" }, map[string]string{"sourcingMode": msgSourcingMode}},
		{"empty mode", func(in *ItemInput) { in.Sourcing.Mode = "" }, map[string]string{"sourcingMode": msgSourcingMode}},
		{"repeated attribute value", func(in *ItemInput) {
			in.AttributeValues = []shared.AttributeValueID{attrValueID(1), attrValueID(1)}
		}, map[string]string{"attributeValueIds": msgValueRepeated}},
		{"too many options", func(in *ItemInput) {
			for n := range 21 {
				in.Options = append(in.Options, validOption(t, byte(n+1)))
			}
		}, map[string]string{"options": msgTooManyOptions}},
		{"option errors carry their path", func(in *ItemInput) {
			bad := validOption(t, 1)
			bad.Name = ""
			bad.Choices[1].Label = "vermelha"
			in.Options = []OptionInput{validOption(t, 2), bad}
		}, map[string]string{"options[1].name": msgOptionNameRequired, "options[1].choices[1].label": msgChoiceRepeated}},
		{"several fields at once", func(in *ItemInput) {
			in.Name = ""
			in.Price = moneyOf(t, 0)
			in.Sourcing.Main = nil
		}, map[string]string{"name": msgItemNameRequired, "priceCents": msgItemPrice, "mainSupplier.supplierId": msgMainSupplier}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := toOrderInput(t)
			tt.mutate(&in)
			_, err := NewItem(tenantID, itemID(1), in)
			wantFields(t, err, tt.want)
		})
	}
}

func TestNewItem_Boundaries(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(in *ItemInput){
		"name of 120 characters":    func(in *ItemInput) { in.Name = repeat("ã", 120) },
		"description of 2000":       func(in *ItemInput) { in.Description = repeat("ã", 2000) },
		"price of one cent":         func(in *ItemInput) { in.Price = moneyOf(t, 1) },
		"highest price":             func(in *ItemInput) { in.Price = moneyOf(t, maxAmountCents) },
		"20 options":                func(in *ItemInput) { in.Options = manyOptions(t, 20) },
		"200 attribute values":      func(in *ItemInput) { in.AttributeValues = manyValues(200) },
		"same-day supplier":         func(in *ItemInput) { in.Sourcing.Main.LeadDays = 0 },
		"longest supplier lead":     func(in *ItemInput) { in.Sourcing.Main.LeadDays = 365 },
		"highest supplier cost":     func(in *ItemInput) { in.Sourcing.Main.Cost = SetCost(moneyOf(t, maxAmountCents)) },
		"free supplier cost":        func(in *ItemInput) { in.Sourcing.Main.Cost = SetCost(moneyOf(t, 0)) },
		"production days of 1":      func(in *ItemInput) { *in = inHouseInput(t); in.Sourcing.ProductionDays = 1 },
		"production days of 365":    func(in *ItemInput) { *in = inHouseInput(t); in.Sourcing.ProductionDays = 365 },
		"in_house with no cost set": func(in *ItemInput) { *in = inHouseInput(t); in.Sourcing.ProductionCost = KeepCost() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := toOrderInput(t)
			mutate(&in)
			if _, err := NewItem(tenantID, itemID(1), in); err != nil {
				t.Errorf("NewItem error = %v, want none", err)
			}
		})
	}
}

func manyOptions(t testing.TB, n int) []OptionInput {
	out := make([]OptionInput, n)
	for i := range out {
		out[i] = validOption(t, byte(i+1))
	}
	return out
}

func manyValues(n int) []shared.AttributeValueID {
	out := make([]shared.AttributeValueID, n)
	for i := range out {
		out[i] = shared.AttributeValueID(shared.NewV7(int64(i+1), [10]byte{byte(i), byte(i >> 8), 1}))
	}
	return out
}

func TestNewItem_SourcingRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   func(t *testing.T) ItemInput
		want map[string]string
	}{
		{"to_order without a supplier", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main = nil
			return in
		}, map[string]string{"mainSupplier.supplierId": msgMainSupplier}},
		{"to_order with a zero supplier id", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main.Supplier = shared.SupplierID{}
			return in
		}, map[string]string{"mainSupplier.supplierId": msgMainSupplier}},
		{"backup with a zero supplier id", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Backup = &SupplierLink{}
			return in
		}, map[string]string{"backupSupplier.supplierId": msgBackupSupplier}},
		{"backup without a main supplier", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main = nil
			in.Sourcing.Backup = link(2, 5, KeepCost())
			return in
		}, map[string]string{"mainSupplier.supplierId": msgMainSupplier}},
		{"backup is the main supplier", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Backup = link(1, 5, KeepCost())
			return in
		}, map[string]string{"backupSupplier.supplierId": msgBackupDiffers}},
		{"lead days too long", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main.LeadDays = 366
			return in
		}, map[string]string{"mainSupplier.leadDays": "Use um prazo de 0 a 365 dias."}},
		{"negative backup lead days", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Backup = link(2, -1, KeepCost())
			return in
		}, map[string]string{"backupSupplier.leadDays": "Use um prazo de 0 a 365 dias."}},
		{"negative supplier cost", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main.Cost = SetCost(moneyOf(t, -1))
			return in
		}, map[string]string{"mainSupplier.costCents": "Informe um custo de zero ou mais."}},
		{"supplier cost too high", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Main.Cost = SetCost(moneyOf(t, maxAmountCents+1))
			return in
		}, map[string]string{"mainSupplier.costCents": msgAmountTooHigh}},
		{"production days on to_order", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.ProductionDays = 2
			return in
		}, map[string]string{"productionDays": msgProductionOnly}},
		{"production cost set on stock", func(t *testing.T) ItemInput {
			in := toOrderInput(t)
			in.Sourcing.Mode = ModeStock
			in.Sourcing.ProductionCost = SetCost(moneyOf(t, 100))
			return in
		}, map[string]string{"productionCostCents": msgProductionOnly}},
		{"in_house with a main supplier", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.Main = link(1, 3, KeepCost())
			return in
		}, map[string]string{"mainSupplier": msgNoSuppliersInHouse}},
		{"in_house with a backup supplier", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.Backup = link(1, 3, KeepCost())
			return in
		}, map[string]string{"backupSupplier": msgNoSuppliersInHouse}},
		{"in_house without production days", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.ProductionDays = 0
			return in
		}, map[string]string{"productionDays": msgProductionDays}},
		{"in_house with 366 production days", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.ProductionDays = 366
			return in
		}, map[string]string{"productionDays": msgProductionRange}},
		{"in_house with negative production days", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.ProductionDays = -2
			return in
		}, map[string]string{"productionDays": msgProductionRange}},
		{"negative production cost", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.ProductionCost = SetCost(moneyOf(t, -1))
			return in
		}, map[string]string{"productionCostCents": msgCostNegative}},
		{"production cost too high", func(t *testing.T) ItemInput {
			in := inHouseInput(t)
			in.Sourcing.ProductionCost = SetCost(moneyOf(t, maxAmountCents+1))
			return in
		}, map[string]string{"productionCostCents": msgAmountTooHigh}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewItem(tenantID, itemID(1), tt.in(t))
			wantFields(t, err, tt.want)
		})
	}
}

func TestNewItem_ProgrammingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tenant shared.TenantID
		id     shared.ItemID
		mutate func(in *ItemInput)
		want   error
	}{
		{"zero tenant", shared.TenantID{}, itemID(1), func(*ItemInput) {}, ErrInvalidItem},
		{"zero id", tenantID, shared.ItemID{}, func(*ItemInput) {}, ErrInvalidItem},
		{"price without a currency", tenantID, itemID(1), func(in *ItemInput) { in.Price = shared.Money{} }, ErrInvalidItem},
		{"cost in another currency", tenantID, itemID(1), func(in *ItemInput) {
			in.Sourcing.Main.Cost = SetCost(mustMoney(t, 100, "USD"))
		}, ErrInvalidItem},
		{"production cost in another currency", tenantID, itemID(1), func(in *ItemInput) {
			*in = inHouseInput(t)
			in.Sourcing.ProductionCost = SetCost(mustMoney(t, 100, "USD"))
		}, ErrInvalidItem},
		{"cost without a currency", tenantID, itemID(1), func(in *ItemInput) {
			in.Sourcing.Main.Cost = SetCost(shared.Money{})
		}, supplier.ErrInvalidSupplierItem},
		{"extra price in another currency", tenantID, itemID(1), func(in *ItemInput) {
			o := validOption(t, 1)
			o.Choices[0].ExtraPrice = mustMoney(t, 100, "USD")
			in.Options = []OptionInput{o}
		}, ErrInvalidOption},
		{"option id used twice", tenantID, itemID(1), func(in *ItemInput) {
			in.Options = []OptionInput{validOption(t, 1), validOption(t, 1)}
		}, ErrInvalidOption},
		{"zero attribute value id", tenantID, itemID(1), func(in *ItemInput) {
			in.AttributeValues = []shared.AttributeValueID{{}}
		}, ErrInvalidItem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := toOrderInput(t)
			tt.mutate(&in)
			_, err := NewItem(tt.tenant, tt.id, in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error %v matches shared.ErrInvalid, but a bug must answer 500", err)
			}
		})
	}
}

func TestItem_InputIsNotShared(t *testing.T) {
	t.Parallel()

	in := toOrderInput(t)
	in.AttributeValues = []shared.AttributeValueID{attrValueID(1)}
	i := newItem(t, 1, in)
	in.AttributeValues[0] = attrValueID(2)
	i.AttributeValueIDs()[0] = attrValueID(3)
	if got := i.AttributeValueIDs()[0]; got != attrValueID(1) {
		t.Errorf("attribute value = %v, want the original", got)
	}
}

func TestItem_Lifecycle(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	if err := i.Publish(); err != nil || !i.Published() {
		t.Fatalf("Publish error = %v, published %v", err, i.Published())
	}
	i.Unpublish()
	if i.Published() {
		t.Error("Unpublish left the item published")
	}

	if err := i.Publish(); err != nil {
		t.Fatal(err)
	}
	i.Archive()
	if !i.Archived() || i.Published() {
		t.Errorf("after Archive: archived %v published %v, want archived and unpublished", i.Archived(), i.Published())
	}
	err := i.Publish()
	if !errors.Is(err, ErrPublishArchived) || !errors.Is(err, shared.ErrConflict) || i.Published() {
		t.Errorf("Publish on an archived item = %v (published %v), want ErrPublishArchived", err, i.Published())
	}
	i.Restore()
	if i.Archived() || i.Published() {
		t.Errorf("after Restore: archived %v published %v, want back, unpublished", i.Archived(), i.Published())
	}
	if err := i.Publish(); err != nil || !i.Published() {
		t.Errorf("Publish after Restore = %v", err)
	}
	if f := i.Facts(); f.Archived {
		t.Error("Facts.Archived = true for a restored item")
	}
}

func TestItem_CheckVersion(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	if err := i.CheckVersion(1); err != nil {
		t.Errorf("CheckVersion(1) = %v, want nil", err)
	}
	if err := i.CheckVersion(2); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("CheckVersion(2) = %v, want ErrVersionConflict", err)
	}
}

func TestItem_Edit(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	err := i.Edit(ItemDetails{Name: " Banco novo ", Description: "d", Price: moneyOf(t, 50000), Options: []OptionInput{validOption(t, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	if i.Name() != "Banco novo" || i.Price().Cents() != 50000 || len(i.Options()) != 1 {
		t.Errorf("after Edit: %q %v %d options", i.Name(), i.Price(), len(i.Options()))
	}
	if i.ReadyDays() != 15 {
		t.Error("Edit changed the sourcing")
	}

	err = i.Edit(ItemDetails{Name: "", Price: moneyOf(t, 50000)})
	wantFields(t, err, map[string]string{"name": msgItemNameRequired})
	if i.Name() != "Banco novo" {
		t.Error("a refused Edit changed the item")
	}
	if err := i.Edit(ItemDetails{Name: "x", Price: mustMoney(t, 100, "USD")}); !errors.Is(err, ErrInvalidItem) {
		t.Errorf("Edit with another currency = %v, want ErrInvalidItem", err)
	}
	if err := i.Edit(ItemDetails{Name: "x", Price: shared.Money{}}); !errors.Is(err, ErrInvalidItem) {
		t.Errorf("Edit without a currency = %v, want ErrInvalidItem", err)
	}
}

func TestItem_Replace(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	in := inHouseInput(t)
	if err := i.Replace(in); err != nil {
		t.Fatal(err)
	}
	if i.SourcingMode() != ModeInHouse || i.Name() != "Bolo de festa" || i.Price().Cents() != 15000 {
		t.Errorf("after Replace: %q %v %q", i.SourcingMode(), i.Price(), i.Name())
	}

	bad := toOrderInput(t)
	bad.Name = ""
	bad.Sourcing.Main = nil
	err := i.Replace(bad)
	wantFields(t, err, map[string]string{"name": msgItemNameRequired, "mainSupplier.supplierId": msgMainSupplier})
	if i.SourcingMode() != ModeInHouse || i.Name() != "Bolo de festa" {
		t.Error("a refused Replace changed the item")
	}

	for name, mutate := range map[string]func(*ItemInput){
		"another currency": func(in *ItemInput) { in.Price = mustMoney(t, 100, "USD"); in.Sourcing.Main.Cost = ClearCost() },
		"no currency":      func(in *ItemInput) { in.Price = shared.Money{} },
	} {
		in := toOrderInput(t)
		mutate(&in)
		if err := i.Replace(in); !errors.Is(err, ErrInvalidItem) {
			t.Errorf("Replace with %s = %v, want ErrInvalidItem", name, err)
		}
	}
}

func TestItem_Photos(t *testing.T) {
	t.Parallel()

	i := newItem(t, 1, toOrderInput(t))
	if _, ok := i.Cover(); ok {
		t.Error("an item without photos has no cover")
	}
	for n := byte(1); n <= 8; n++ {
		if err := i.AddPhoto(newPhoto(t, n)); err != nil {
			t.Fatalf("AddPhoto %d: %v", n, err)
		}
	}
	if cover, _ := i.Cover(); cover.ID() != photoID(1) {
		t.Errorf("cover = %v, want the first photo", cover.ID())
	}
	if err := i.AddPhoto(newPhoto(t, 9)); !errors.Is(err, ErrPhotoLimit) || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("ninth photo error = %v, want ErrPhotoLimit", err)
	}
	if len(i.Photos()) != 8 {
		t.Errorf("photos = %d, want 8", len(i.Photos()))
	}
}
