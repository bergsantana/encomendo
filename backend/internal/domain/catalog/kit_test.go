package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The items of the "Kit Scrambler CB 500X" fixtures. Item 1 is the leather
// seat, the slowest at 15 days.
const (
	seat      = 1
	exhaust   = 2
	handlebar = 3
	cheapSeat = 4
	richSeat  = 5
	archived  = 6
	highBar   = 7
)

func facts(t testing.TB, price int64, ready int, cost int64) ItemFacts {
	return ItemFacts{Price: moneyOf(t, price), ReadyDays: ready, Cost: costPtr(t, cost)}
}

func validLookup(t testing.TB) ItemLookup {
	lookup := ItemLookup{
		itemID(seat):      facts(t, 40000, 15, 25000),
		itemID(exhaust):   facts(t, 30000, 7, 18000),
		itemID(handlebar): facts(t, 10000, 2, 6000),
		itemID(cheapSeat): facts(t, 25000, 5, 15000),
		itemID(richSeat):  facts(t, 55000, 20, 30000),
		itemID(highBar):   facts(t, 15000, 3, 9000),
		itemID(archived):  facts(t, 100, 1, 50),
	}
	f := lookup[itemID(archived)]
	f.Archived = true
	lookup[itemID(archived)] = f
	return lookup
}

// validKitInput is the Kit Scrambler: a seat (swappable for two seats), an
// exhaust and two handlebars (swappable for a high bar), at R$ 700,00 for
// R$ 900,00 of items.
func validKitInput(t testing.TB) KitInput {
	return KitInput{
		Name:     "Kit Scrambler CB 500X",
		Price:    moneyOf(t, 70000),
		SwapMode: SwapKeepDiscount,
		Items: []KitItem{
			{Item: itemID(seat), Quantity: 1, Swappable: true, Alternatives: []shared.ItemID{itemID(cheapSeat), itemID(richSeat)}},
			{Item: itemID(exhaust), Quantity: 1},
			{Item: itemID(handlebar), Quantity: 2, Swappable: true, Alternatives: []shared.ItemID{itemID(highBar)}},
		},
	}
}

func newKit(t testing.TB, id byte, in KitInput, lookup ItemLookup) *Kit {
	t.Helper()
	k, err := NewKit(tenantID, kitID(id), in, lookup)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func intPtr(v int) *int { return &v }

func TestNewKit(t *testing.T) {
	t.Parallel()

	in := validKitInput(t)
	in.Name = "  Kit Scrambler CB 500X  "
	in.Description = " Tudo para a Scrambler. "
	in.SwapMode = ""
	in.AssemblyDays = intPtr(4)
	in.AttributeValues = []shared.AttributeValueID{attrValueID(1)}
	k := newKit(t, 1, in, validLookup(t))

	if k.TenantID() != tenantID || k.ID() != kitID(1) || k.Version() != 1 || k.Published() || k.Archived() {
		t.Errorf("kit = %v %v v%d published %v archived %v", k.TenantID(), k.ID(), k.Version(), k.Published(), k.Archived())
	}
	if k.Name() != "Kit Scrambler CB 500X" || k.Description() != "Tudo para a Scrambler." {
		t.Errorf("name %q description %q, want both trimmed", k.Name(), k.Description())
	}
	if k.SwapMode() != SwapKeepDiscount {
		t.Errorf("SwapMode = %q, want the default keep", k.SwapMode())
	}
	if v, ok := k.AssemblyDays(); !ok || v != 4 {
		t.Errorf("AssemblyDays = %d, %v, want 4 and true", v, ok)
	}
	if _, ok := k.SafetyMarginDays(); ok {
		t.Error("SafetyMarginDays reports an override, want the store default")
	}
	if k.Price().Cents() != 70000 || len(k.Items()) != 3 || len(k.AttributeValueIDs()) != 1 {
		t.Errorf("price %v items %d attributes %d", k.Price(), len(k.Items()), len(k.AttributeValueIDs()))
	}
}

func TestNewKit_CopiesItsInput(t *testing.T) {
	t.Parallel()

	in := validKitInput(t)
	in.AssemblyDays = intPtr(4)
	k := newKit(t, 1, in, validLookup(t))
	*in.AssemblyDays = 9
	in.Items[0].Alternatives[0] = itemID(highBar)
	k.Items()[0].Alternatives[0] = itemID(highBar)
	if v, _ := k.AssemblyDays(); v != 4 {
		t.Errorf("AssemblyDays = %d, want 4", v)
	}
	if got := k.Items()[0].Alternatives[0]; got != itemID(cheapSeat) {
		t.Errorf("first alternative = %v, want the original", got)
	}
}

func TestNewKit_Boundaries(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	for name, mutate := range map[string]func(in *KitInput){
		"swap mode lose":         func(in *KitInput) { in.SwapMode = SwapLoseDiscount },
		"zero days overrides":    func(in *KitInput) { in.AssemblyDays, in.SafetyMarginDays = intPtr(0), intPtr(0) },
		"60 days overrides":      func(in *KitInput) { in.AssemblyDays, in.SafetyMarginDays = intPtr(60), intPtr(60) },
		"quantity 99":            func(in *KitInput) { in.Items[1].Quantity = 99 },
		"a single item":          func(in *KitInput) { in.Items = in.Items[1:2] },
		"price of one cent":      func(in *KitInput) { in.Price = moneyOf(t, 1) },
		"price above the sum":    func(in *KitInput) { in.Price = moneyOf(t, 500000) },
		"name of 120 characters": func(in *KitInput) { in.Name = repeat("ã", 120) },
		"description of 2000":    func(in *KitInput) { in.Description = repeat("ã", 2000) },
		"200 attribute values":   func(in *KitInput) { in.AttributeValues = manyValues(200) },
		"a single alternative":   func(in *KitInput) { in.Items[0].Alternatives = []shared.ItemID{itemID(cheapSeat)} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			mutate(&in)
			if _, err := NewKit(tenantID, kitID(1), in, lookup); err != nil {
				t.Errorf("NewKit error = %v, want none", err)
			}
		})
	}
}

func TestNewKit_Validation(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	tests := []struct {
		name   string
		mutate func(in *KitInput)
		want   map[string]string
	}{
		{"blank name", func(in *KitInput) { in.Name = " " }, map[string]string{"name": msgKitNameRequired}},
		{"name over 120", func(in *KitInput) { in.Name = repeat("a", 121) }, map[string]string{"name": msgNameTooLong}},
		{"name with a control character", func(in *KitInput) { in.Name = "a\nb" }, map[string]string{"name": msgNameControl}},
		{"description over 2000", func(in *KitInput) { in.Description = repeat("a", 2001) }, map[string]string{"description": msgDescriptionTooLong}},
		{"zero price", func(in *KitInput) { in.Price = moneyOf(t, 0) }, map[string]string{"priceCents": msgKitPrice}},
		{"negative price", func(in *KitInput) { in.Price = moneyOf(t, -1) }, map[string]string{"priceCents": msgKitPrice}},
		{"price too high", func(in *KitInput) { in.Price = moneyOf(t, maxAmountCents+1) }, map[string]string{"priceCents": msgAmountTooHigh}},
		{"unknown swap mode", func(in *KitInput) { in.SwapMode = "both" }, map[string]string{"swapDiscount": msgSwapMode}},
		{"assembly over 60", func(in *KitInput) { in.AssemblyDays = intPtr(61) }, map[string]string{"assemblyDays": msgDays0To60}},
		{"negative margin", func(in *KitInput) { in.SafetyMarginDays = intPtr(-1) }, map[string]string{"safetyMarginDays": msgDays0To60}},
		{"no items", func(in *KitInput) { in.Items = nil }, map[string]string{"items": msgNoKitItems}},
		{"51 items", func(in *KitInput) { in.Items = make([]KitItem, 51) }, map[string]string{"items": msgTooManyKitItems}},
		{"quantity 0", func(in *KitInput) { in.Items[1].Quantity = 0 }, map[string]string{"items[1].quantity": msgQuantity}},
		{"quantity 100", func(in *KitInput) { in.Items[1].Quantity = 100 }, map[string]string{"items[1].quantity": msgQuantity}},
		{"item twice", func(in *KitInput) { in.Items[1].Item = itemID(seat) }, map[string]string{"items[1].itemId": msgKitItemRepeated}},
		{"item not in the lookup", func(in *KitInput) { in.Items[1].Item = itemID(99) }, map[string]string{"items[1].itemId": msgKitItemUnknown}},
		{"swappable without alternatives", func(in *KitInput) { in.Items[0].Alternatives = nil }, map[string]string{"items[0].alternativeItemIds": msgNoAlternatives}},
		{"alternatives on a fixed item", func(in *KitInput) { in.Items[1].Alternatives = []shared.ItemID{itemID(cheapSeat)} }, map[string]string{"items[1].alternativeItemIds": msgAlternativesOff}},
		{"item as its own alternative", func(in *KitInput) { in.Items[0].Alternatives = []shared.ItemID{itemID(seat)} }, map[string]string{"items[0].alternativeItemIds[0]": msgAlternativeSelf}},
		{"alternative not in the lookup", func(in *KitInput) { in.Items[0].Alternatives[1] = itemID(99) }, map[string]string{"items[0].alternativeItemIds[1]": msgKitItemUnknown}},
		{"alternative twice", func(in *KitInput) { in.Items[0].Alternatives[1] = itemID(cheapSeat) }, map[string]string{"items[0].alternativeItemIds[1]": msgAlternativeDup}},
		{"51 alternatives", func(in *KitInput) { in.Items[0].Alternatives = make([]shared.ItemID, 51) }, map[string]string{"items[0].alternativeItemIds": msgTooManyAlts}},
		{"repeated attribute value", func(in *KitInput) {
			in.AttributeValues = []shared.AttributeValueID{attrValueID(1), attrValueID(1)}
		}, map[string]string{"attributeValueIds": msgValueRepeated}},
		{"several at once", func(in *KitInput) {
			in.Name = ""
			in.Price = moneyOf(t, 0)
			in.Items[1].Quantity = 0
		}, map[string]string{"name": msgKitNameRequired, "priceCents": msgKitPrice, "items[1].quantity": msgQuantity}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			tt.mutate(&in)
			_, err := NewKit(tenantID, kitID(1), in, lookup)
			wantFields(t, err, tt.want)
		})
	}
}

func TestNewKit_ProgrammingErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tenant shared.TenantID
		id     shared.KitID
		mutate func(in *KitInput)
	}{
		{"zero tenant", shared.TenantID{}, kitID(1), func(*KitInput) {}},
		{"zero id", tenantID, shared.KitID{}, func(*KitInput) {}},
		{"price without a currency", tenantID, kitID(1), func(in *KitInput) { in.Price = shared.Money{} }},
		{"zero attribute value id", tenantID, kitID(1), func(in *KitInput) { in.AttributeValues = []shared.AttributeValueID{{}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			tt.mutate(&in)
			_, err := NewKit(tt.tenant, tt.id, in, validLookup(t))
			if !errors.Is(err, ErrInvalidKit) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalidKit and not a 400", err)
			}
		})
	}
}

func TestNewKit_RefusesArchivedItems(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	tests := []struct {
		name   string
		mutate func(in *KitInput)
		want   []shared.ItemID
	}{
		{"a member", func(in *KitInput) { in.Items[1].Item = itemID(archived) }, []shared.ItemID{itemID(archived)}},
		{"an alternative", func(in *KitInput) { in.Items[0].Alternatives[1] = itemID(archived) }, []shared.ItemID{itemID(archived)}},
		{"listed once when it is both", func(in *KitInput) {
			in.Items[1].Item = itemID(archived)
			in.Items[0].Alternatives[1] = itemID(archived)
		}, []shared.ItemID{itemID(archived)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			tt.mutate(&in)
			_, err := NewKit(tenantID, kitID(1), in, lookup)
			var ae *ArchivedItemsError
			if !errors.As(err, &ae) || len(ae.Items) != len(tt.want) || ae.Items[0] != tt.want[0] {
				t.Fatalf("error = %v, want ArchivedItemsError listing %v", err, tt.want)
			}
			if !errors.Is(err, ErrItemArchived) || !errors.Is(err, shared.ErrConflict) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error %v must match ErrItemArchived and ErrConflict (409), not ErrInvalid", err)
			}
		})
	}
}

func TestNewKit_ListsEveryArchivedItemInKitOrder(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	for _, id := range []byte{exhaust, cheapSeat} {
		f := lookup[itemID(id)]
		f.Archived = true
		lookup[itemID(id)] = f
	}
	_, err := NewKit(tenantID, kitID(1), validKitInput(t), lookup)
	var ae *ArchivedItemsError
	if !errors.As(err, &ae) || len(ae.Items) != 2 || ae.Items[0] != itemID(cheapSeat) || ae.Items[1] != itemID(exhaust) {
		t.Fatalf("error = %v, want [cheapSeat exhaust], in the order the kit holds them", err)
	}
	if ae.Error() == "" {
		t.Error("an ArchivedItemsError must describe itself")
	}
}

func TestNewKit_ValidationErrorsComeBeforeArchivedOnes(t *testing.T) {
	t.Parallel()

	in := validKitInput(t)
	in.Name = ""
	in.Items[1].Item = itemID(archived)
	_, err := NewKit(tenantID, kitID(1), in, validLookup(t))
	wantFields(t, err, map[string]string{"name": msgKitNameRequired})
}

func TestKit_Replace(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	k := newKit(t, 1, validKitInput(t), lookup)
	in := validKitInput(t)
	in.Name = "Kit novo"
	in.SwapMode = SwapLoseDiscount
	in.Items = in.Items[1:2]
	if err := k.Replace(in, lookup); err != nil {
		t.Fatal(err)
	}
	if k.Name() != "Kit novo" || k.SwapMode() != SwapLoseDiscount || len(k.Items()) != 1 {
		t.Errorf("after Replace: %q %q %d items", k.Name(), k.SwapMode(), len(k.Items()))
	}

	bad := validKitInput(t)
	bad.Name = ""
	wantFields(t, k.Replace(bad, lookup), map[string]string{"name": msgKitNameRequired})
	if k.Name() != "Kit novo" {
		t.Error("a refused Replace changed the kit")
	}
	bad = validKitInput(t)
	bad.Items[1].Item = itemID(archived)
	if err := k.Replace(bad, lookup); !errors.Is(err, ErrItemArchived) || k.Name() != "Kit novo" {
		t.Errorf("Replace with an archived item = %v, kit %q", err, k.Name())
	}
}

func TestKit_Lifecycle(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	k := newKit(t, 1, validKitInput(t), lookup)
	if err := k.Publish(); err != nil || !k.Published() {
		t.Fatalf("Publish = %v, published %v", err, k.Published())
	}
	k.Unpublish()
	if k.Published() {
		t.Error("Unpublish left the kit published")
	}
	if err := k.Publish(); err != nil {
		t.Fatal(err)
	}
	k.Archive()
	if !k.Archived() || k.Published() {
		t.Errorf("after Archive: archived %v published %v", k.Archived(), k.Published())
	}
	if err := k.Publish(); !errors.Is(err, ErrPublishArchived) || k.Published() {
		t.Errorf("Publish on an archived kit = %v, published %v", err, k.Published())
	}
	if err := k.Restore(lookup); err != nil || k.Archived() || k.Published() {
		t.Errorf("Restore = %v, archived %v published %v, want back and unpublished", err, k.Archived(), k.Published())
	}
	if err := k.CheckVersion(1); err != nil {
		t.Errorf("CheckVersion(1) = %v", err)
	}
	if err := k.CheckVersion(2); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("CheckVersion(2) = %v, want ErrVersionConflict", err)
	}
}

func TestKit_Restore_RefusesArchivedItems(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	for name, id := range map[string]byte{"a member": exhaust, "an alternative": richSeat} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			k := newKit(t, 1, validKitInput(t), lookup)
			k.Archive()
			now := validLookup(t)
			f := now[itemID(id)]
			f.Archived = true
			now[itemID(id)] = f

			err := k.Restore(now)
			var ae *ArchivedItemsError
			if !errors.As(err, &ae) || len(ae.Items) != 1 || ae.Items[0] != itemID(id) || !errors.Is(err, ErrItemArchived) {
				t.Fatalf("Restore = %v, want ArchivedItemsError for %v", err, itemID(id))
			}
			if !k.Archived() {
				t.Error("a refused Restore must leave the kit archived")
			}
		})
	}
	t.Run("an item missing from the lookup", func(t *testing.T) {
		t.Parallel()
		k := newKit(t, 1, validKitInput(t), lookup)
		k.Archive()
		partial := validLookup(t)
		delete(partial, itemID(richSeat))
		if err := k.Restore(partial); !errors.Is(err, ErrItemMissing) || !k.Archived() {
			t.Errorf("Restore = %v, archived %v, want ErrItemMissing and still archived", err, k.Archived())
		}
	})
}

func TestKit_Totals(t *testing.T) {
	t.Parallel()

	k := newKit(t, 1, validKitInput(t), validLookup(t))
	lookup := validLookup(t)

	total, err := k.SingleItemsTotal(lookup)
	if err != nil || total.Cents() != 90000 {
		t.Errorf("SingleItemsTotal = %v, %v, want 90000 (40000 + 30000 + 2 × 10000)", total, err)
	}
	discount, err := k.Discount(lookup)
	if err != nil || discount.Cents() != 20000 {
		t.Errorf("Discount = %v, %v, want 20000", discount, err)
	}

	pricey := validKitInput(t)
	pricey.Price = moneyOf(t, 95000)
	d, err := newKit(t, 2, pricey, lookup).Discount(lookup)
	if err != nil || d.Cents() != -5000 {
		t.Errorf("Discount of a kit above its items = %v, %v, want -5000", d, err)
	}
}

func TestKit_Days_KitScramblerCB500X(t *testing.T) {
	t.Parallel()

	// docs/product/design-document.md, Deadline formula: the leather seat
	// takes 15 days, assembly 3 and the safety margin 2: 20 days.
	k := newKit(t, 1, validKitInput(t), validLookup(t))
	got, err := k.Days(validLookup(t), 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := (KitDays{SlowestItem: 15, Assembly: 3, SafetyMargin: 2, Total: 20}); got != want {
		t.Errorf("Days = %+v, want %+v", got, want)
	}
}

func TestKit_Days(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		assembly, margin *int
		defA, defM       int
		want             KitDays
	}{
		{"store defaults", nil, nil, 3, 2, KitDays{15, 3, 2, 20}},
		{"own assembly wins", intPtr(5), nil, 3, 2, KitDays{15, 5, 2, 22}},
		{"own margin wins", nil, intPtr(0), 3, 2, KitDays{15, 3, 0, 18}},
		{"both own, zero", intPtr(0), intPtr(0), 3, 2, KitDays{15, 0, 0, 15}},
		{"no defaults", nil, nil, 0, 0, KitDays{15, 0, 0, 15}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			in.AssemblyDays, in.SafetyMarginDays = tt.assembly, tt.margin
			got, err := newKit(t, 1, in, validLookup(t)).Days(validLookup(t), tt.defA, tt.defM)
			if err != nil || got != tt.want {
				t.Errorf("Days = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestKit_Days_Errors(t *testing.T) {
	t.Parallel()

	k := newKit(t, 1, validKitInput(t), validLookup(t))
	if _, err := k.Days(validLookup(t), -1, 0); !errors.Is(err, schedule.ErrNegativeDays) {
		t.Errorf("negative default = %v, want ErrNegativeDays", err)
	}
	partial := validLookup(t)
	delete(partial, itemID(exhaust))
	if _, err := k.Days(partial, 0, 0); !errors.Is(err, ErrItemMissing) || !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("missing item = %v, want ErrItemMissing", err)
	}
}

func TestKit_CostAndMargin(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	k := newKit(t, 1, validKitInput(t), lookup)

	cost, err := k.Cost(lookup)
	if err != nil || cost == nil || cost.Cents() != 55000 {
		t.Fatalf("Cost = %v, %v, want 55000 (25000 + 18000 + 2 × 6000)", cost, err)
	}
	m, ok, err := k.Margin(lookup)
	if err != nil || !ok || m != (Margin{Cents: 15000, BasisPoints: 2142}) {
		t.Errorf("Margin = %+v, %v, %v, want 15000 and 2142 (rounded toward zero)", m, ok, err)
	}

	noCost := validLookup(t)
	f := noCost[itemID(exhaust)]
	f.Cost = nil
	noCost[itemID(exhaust)] = f
	if cost, err := k.Cost(noCost); err != nil || cost != nil {
		t.Errorf("Cost with an item without a cost = %v, %v, want none", cost, err)
	}
	if _, ok, err := k.Margin(noCost); err != nil || ok {
		t.Errorf("Margin with an item without a cost = %v, %v, want none", ok, err)
	}

	partial := validLookup(t)
	delete(partial, itemID(seat))
	if _, _, err := k.Margin(partial); !errors.Is(err, ErrItemMissing) {
		t.Errorf("Margin with a missing item = %v, want ErrItemMissing", err)
	}
}

func TestKit_Totals_MoneyErrors(t *testing.T) {
	t.Parallel()

	k := newKit(t, 1, validKitInput(t), validLookup(t))
	usd := validLookup(t)
	usd[itemID(seat)] = ItemFacts{Price: mustMoney(t, 100, "USD"), ReadyDays: 1}
	if _, err := k.SingleItemsTotal(usd); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Errorf("SingleItemsTotal across currencies = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := k.Discount(usd); err == nil {
		t.Error("Discount across currencies must fail")
	}

	huge := validLookup(t)
	huge[itemID(handlebar)] = ItemFacts{Price: moneyOf(t, 1<<62), ReadyDays: 1}
	if _, err := k.SingleItemsTotal(huge); !errors.Is(err, shared.ErrMoneyOverflow) {
		t.Errorf("SingleItemsTotal that overflows = %v, want ErrMoneyOverflow", err)
	}
	huge[itemID(handlebar)] = ItemFacts{Price: moneyOf(t, 1<<61), ReadyDays: 1}
	huge[itemID(seat)] = ItemFacts{Price: moneyOf(t, 1<<61), ReadyDays: 1}
	huge[itemID(exhaust)] = ItemFacts{Price: moneyOf(t, 1<<61), ReadyDays: 1}
	if _, err := k.SingleItemsTotal(huge); !errors.Is(err, shared.ErrMoneyOverflow) {
		t.Errorf("a sum that overflows = %v, want ErrMoneyOverflow", err)
	}
	if _, err := k.SingleItemsTotal(ItemLookup{}); !errors.Is(err, ErrItemMissing) {
		t.Errorf("SingleItemsTotal with no facts = %v, want ErrItemMissing", err)
	}
}

func TestRestoreKit(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	in := validKitInput(t)
	rec := KitRecord{
		Tenant: tenantID, ID: kitID(1), Name: in.Name, Price: in.Price, SwapMode: SwapLoseDiscount,
		AssemblyDays: intPtr(2), Items: in.Items, Attributes: []shared.AttributeValueID{attrValueID(1)},
		Photos: []Photo{newPhoto(t, 1)}, Published: true, Version: 3,
	}
	k, err := RestoreKit(rec, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if k.Version() != 3 || !k.Published() || k.SwapMode() != SwapLoseDiscount || len(k.Photos()) != 1 {
		t.Errorf("restored: v%d published %v mode %q photos %d", k.Version(), k.Published(), k.SwapMode(), len(k.Photos()))
	}

	// An archived kit keeps the items it held, even once they are archived.
	now := validLookup(t)
	f := now[itemID(exhaust)]
	f.Archived = true
	now[itemID(exhaust)] = f
	rec.Published, rec.Archived = false, true
	if k, err := RestoreKit(rec, now); err != nil || !k.Archived() {
		t.Errorf("RestoreKit of an archived kit = %v, %v", k, err)
	}
	// A live kit never holds one.
	rec.Archived = false
	if _, err := RestoreKit(rec, now); !errors.Is(err, ErrInvalidKit) {
		t.Errorf("RestoreKit of a live kit with an archived item = %v, want ErrInvalidKit", err)
	}
}

func TestRestoreKit_RefusesABrokenRecord(t *testing.T) {
	t.Parallel()

	good := func() KitRecord {
		in := validKitInput(t)
		return KitRecord{Tenant: tenantID, ID: kitID(1), Name: in.Name, Price: in.Price, SwapMode: SwapKeepDiscount, Items: in.Items, Version: 1}
	}
	tests := map[string]func(r *KitRecord){
		"version 0":             func(r *KitRecord) { r.Version = 0 },
		"untrimmed name":        func(r *KitRecord) { r.Name += " " },
		"untrimmed description": func(r *KitRecord) { r.Description = " d" },
		"empty swap mode":       func(r *KitRecord) { r.SwapMode = "" },
		"archived and published": func(r *KitRecord) {
			r.Archived, r.Published = true, true
		},
		"blank name":      func(r *KitRecord) { r.Name = "" },
		"unknown item":    func(r *KitRecord) { r.Items = []KitItem{{Item: itemID(99), Quantity: 1}} },
		"repeated photos": func(r *KitRecord) { r.Photos = []Photo{newPhoto(t, 1), newPhoto(t, 1)} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := good()
			mutate(&r)
			_, err := RestoreKit(r, validLookup(t))
			if !errors.Is(err, ErrInvalidKit) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalidKit and not a 400", err)
			}
		})
	}
	if _, err := RestoreKit(good(), validLookup(t)); err != nil {
		t.Errorf("the base record must be valid: %v", err)
	}
}
