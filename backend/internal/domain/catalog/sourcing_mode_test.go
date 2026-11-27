package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

func TestSourcingMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode  SourcingMode
		valid bool
		buys  bool
	}{
		{ModeToOrder, true, true},
		{ModeStock, true, true},
		{ModeInHouse, true, false},
		{"", false, false},
		{"To_Order", false, false},
		{"dropship", false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()
			if tt.mode.Valid() != tt.valid || tt.mode.NeedsSupplier() != (tt.valid && tt.buys) {
				t.Errorf("Valid = %v, NeedsSupplier = %v, want %v and %v", tt.mode.Valid(), tt.mode.NeedsSupplier(), tt.valid, tt.buys)
			}
		})
	}
}

// itemWithSuppliers returns a to_order item whose main is supplier 1 (cost
// R$ 100,00) and whose backup is supplier 2 (cost R$ 200,00).
func itemWithSuppliers(t *testing.T) *Item {
	t.Helper()
	in := toOrderInput(t)
	in.Sourcing.Main = link(1, 10, SetCost(moneyOf(t, 10000)))
	in.Sourcing.Backup = link(2, 20, SetCost(moneyOf(t, 20000)))
	return newItem(t, 1, in)
}

// costsOf returns the costs of main and backup in cents, -1 for none, and 0
// for a missing supplier item (reported as "gone").
func costsOf(i *Item) (main, backup int64, ids [2]shared.SupplierID) {
	main, backup = -2, -2
	if m, ok := i.MainSupplier(); ok {
		ids[0] = m.SupplierID()
		main = centsOrNone(m.Cost())
	}
	if b, ok := i.BackupSupplier(); ok {
		ids[1] = b.SupplierID()
		backup = centsOrNone(b.Cost())
	}
	return main, backup, ids
}

func centsOrNone(m *shared.Money) int64 {
	if m == nil {
		return -1
	}
	return m.Cents()
}

// TestItem_ReplaceSourcing_CostFollowsTheSupplier covers ADR 0025: a "keep"
// takes the cost of the same supplier, never of the same slot, so a write
// that leaves costs out cannot move an owner's cost to another supplier.
func TestItem_ReplaceSourcing_CostFollowsTheSupplier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         func(t *testing.T) SourcingInput
		wantMain   int64 // cents, -1 none
		wantBackup int64 // cents, -1 none, -2 no backup
		wantIDs    [2]shared.SupplierID
	}{
		{"keep everything keeps every cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(1, 11, KeepCost()), Backup: link(2, 21, KeepCost())}
		}, 10000, 20000, [2]shared.SupplierID{supplierID(1), supplierID(2)}},
		{"a new main supplier starts with no cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(3, 5, KeepCost()), Backup: link(2, 21, KeepCost())}
		}, -1, 20000, [2]shared.SupplierID{supplierID(3), supplierID(2)}},
		{"a new backup supplier starts with no cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(1, 10, KeepCost()), Backup: link(3, 5, KeepCost())}
		}, 10000, -1, [2]shared.SupplierID{supplierID(1), supplierID(3)}},
		{"swapping main and backup keeps each supplier's own cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(2, 20, KeepCost()), Backup: link(1, 10, KeepCost())}
		}, 20000, 10000, [2]shared.SupplierID{supplierID(2), supplierID(1)}},
		{"promoting the backup keeps its cost and drops the old main's", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(2, 20, KeepCost())}
		}, 20000, -2, [2]shared.SupplierID{supplierID(2), {}}},
		{"unlinking the backup drops its cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(1, 10, KeepCost())}
		}, 10000, -2, [2]shared.SupplierID{supplierID(1), {}}},
		{"clear removes the cost of that supplier only", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(1, 10, ClearCost()), Backup: link(2, 20, KeepCost())}
		}, -1, 20000, [2]shared.SupplierID{supplierID(1), supplierID(2)}},
		{"set replaces the cost of that supplier only", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(1, 10, SetCost(moneyOf(t, 12345))), Backup: link(2, 20, KeepCost())}
		}, 12345, 20000, [2]shared.SupplierID{supplierID(1), supplierID(2)}},
		{"the zero CostChange keeps", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: &SupplierLink{Supplier: supplierID(1), LeadDays: 10}}
		}, 10000, -2, [2]shared.SupplierID{supplierID(1), {}}},
		{"changing to_order into stock keeps the supplier's cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeStock, Main: link(1, 10, KeepCost()), Backup: link(2, 20, KeepCost())}
		}, 10000, 20000, [2]shared.SupplierID{supplierID(1), supplierID(2)}},
		{"a swap with a new supplier in the old slot never inherits the slot's cost", func(t *testing.T) SourcingInput {
			return SourcingInput{Mode: ModeToOrder, Main: link(4, 7, KeepCost()), Backup: link(1, 10, KeepCost())}
		}, -1, 10000, [2]shared.SupplierID{supplierID(4), supplierID(1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			i := itemWithSuppliers(t)
			if err := i.ReplaceSourcing(tt.in(t)); err != nil {
				t.Fatal(err)
			}
			main, backup, ids := costsOf(i)
			if main != tt.wantMain || backup != tt.wantBackup || ids != tt.wantIDs {
				t.Errorf("main %d backup %d suppliers %v, want %d %d %v", main, backup, ids, tt.wantMain, tt.wantBackup, tt.wantIDs)
			}
		})
	}
}

func TestItem_ReplaceSourcing_RelinkingADroppedSupplierStartsWithNoCost(t *testing.T) {
	t.Parallel()

	i := itemWithSuppliers(t)
	only1 := SourcingInput{Mode: ModeToOrder, Main: link(1, 10, KeepCost())}
	if err := i.ReplaceSourcing(only1); err != nil {
		t.Fatal(err)
	}
	back := SourcingInput{Mode: ModeToOrder, Main: link(1, 10, KeepCost()), Backup: link(2, 20, KeepCost())}
	if err := i.ReplaceSourcing(back); err != nil {
		t.Fatal(err)
	}
	if main, backup, _ := costsOf(i); main != 10000 || backup != -1 {
		t.Errorf("main %d backup %d, want 10000 and none: supplier 2 was unlinked in between", main, backup)
	}
}

func TestItem_ReplaceSourcing_ReadyDaysFollowTheMainSupplier(t *testing.T) {
	t.Parallel()

	i := itemWithSuppliers(t)
	if err := i.ReplaceSourcing(SourcingInput{Mode: ModeToOrder, Main: link(2, 20, KeepCost()), Backup: link(1, 10, KeepCost())}); err != nil {
		t.Fatal(err)
	}
	if i.ReadyDays() != 20 {
		t.Errorf("ReadyDays = %d, want the new main supplier's 20", i.ReadyDays())
	}
}

func TestItem_ReplaceSourcing_InHouseCosts(t *testing.T) {
	t.Parallel()

	inHouse := func(cost CostChange) SourcingInput {
		return SourcingInput{Mode: ModeInHouse, ProductionDays: 4, ProductionCost: cost}
	}
	t.Run("keep stays in_house keeps the production cost", func(t *testing.T) {
		t.Parallel()
		i := newItem(t, 1, inHouseInput(t))
		if err := i.ReplaceSourcing(inHouse(KeepCost())); err != nil {
			t.Fatal(err)
		}
		if pc := i.ProductionCost(); pc == nil || pc.Cents() != 6000 || i.ProductionDays() != 4 {
			t.Errorf("production cost %v days %d, want 6000 and 4", pc, i.ProductionDays())
		}
	})
	t.Run("clear removes it", func(t *testing.T) {
		t.Parallel()
		i := newItem(t, 1, inHouseInput(t))
		if err := i.ReplaceSourcing(inHouse(ClearCost())); err != nil {
			t.Fatal(err)
		}
		if i.ProductionCost() != nil || i.Cost() != nil {
			t.Error("production cost was not cleared")
		}
	})
	t.Run("set replaces it", func(t *testing.T) {
		t.Parallel()
		i := newItem(t, 1, inHouseInput(t))
		if err := i.ReplaceSourcing(inHouse(SetCost(moneyOf(t, 7000)))); err != nil {
			t.Fatal(err)
		}
		if pc := i.ProductionCost(); pc == nil || pc.Cents() != 7000 {
			t.Errorf("production cost = %v, want 7000", pc)
		}
	})
	t.Run("leaving in_house drops the production cost and coming back starts with none", func(t *testing.T) {
		t.Parallel()
		i := newItem(t, 1, inHouseInput(t))
		if err := i.ReplaceSourcing(SourcingInput{Mode: ModeToOrder, Main: link(1, 5, KeepCost())}); err != nil {
			t.Fatal(err)
		}
		if i.ProductionCost() != nil || i.ProductionDays() != 0 || i.Cost() != nil {
			t.Errorf("after leaving in_house: production cost %v days %d cost %v, want none", i.ProductionCost(), i.ProductionDays(), i.Cost())
		}
		if err := i.ReplaceSourcing(inHouse(KeepCost())); err != nil {
			t.Fatal(err)
		}
		if i.ProductionCost() != nil {
			t.Errorf("coming back to in_house kept %v, want none", i.ProductionCost())
		}
	})
	t.Run("leaving in_house never gives a supplier the production cost", func(t *testing.T) {
		t.Parallel()
		i := newItem(t, 1, inHouseInput(t))
		if err := i.ReplaceSourcing(SourcingInput{Mode: ModeStock, Main: link(1, 5, KeepCost())}); err != nil {
			t.Fatal(err)
		}
		if main, _, _ := costsOf(i); main != -1 {
			t.Errorf("main cost = %d, want none", main)
		}
	})
	t.Run("leaving a supplier mode for in_house drops the supplier costs", func(t *testing.T) {
		t.Parallel()
		i := itemWithSuppliers(t)
		if err := i.ReplaceSourcing(inHouse(KeepCost())); err != nil {
			t.Fatal(err)
		}
		if i.Cost() != nil || i.ProductionCost() != nil {
			t.Error("an in_house item must start with no cost")
		}
		if _, ok := i.MainSupplier(); ok {
			t.Error("an in_house item has no supplier")
		}
	})
}

func TestItem_ReplaceSourcing_RefusedChangesNothing(t *testing.T) {
	t.Parallel()

	i := itemWithSuppliers(t)
	err := i.ReplaceSourcing(SourcingInput{Mode: ModeToOrder, Main: link(1, 400, KeepCost()), Backup: link(1, 5, KeepCost())})
	wantFields(t, err, map[string]string{
		"mainSupplier.leadDays":     "Use um prazo de 0 a 365 dias.",
		"backupSupplier.supplierId": msgBackupDiffers,
	})
	main, backup, ids := costsOf(i)
	if main != 10000 || backup != 20000 || ids != [2]shared.SupplierID{supplierID(1), supplierID(2)} || i.ReadyDays() != 10 {
		t.Errorf("a refused write changed the item: %d %d %v ready %d", main, backup, ids, i.ReadyDays())
	}
	if err := i.ReplaceSourcing(SourcingInput{Mode: ModeToOrder, Main: link(1, 5, SetCost(mustMoney(t, 1, "USD")))}); !errors.Is(err, ErrInvalidItem) {
		t.Errorf("a cost in another currency = %v, want ErrInvalidItem", err)
	}
}

func TestRestoreItem(t *testing.T) {
	t.Parallel()

	main, err := supplier.NewSupplierItem(supplierID(1), supplier.RoleMain, 15, costPtr(t, 25000))
	if err != nil {
		t.Fatal(err)
	}
	backup, err := supplier.NewSupplierItem(supplierID(2), supplier.RoleBackup, 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	photo := newPhoto(t, 1)
	rec := ItemRecord{
		Tenant: tenantID, ID: itemID(1), Name: "Banco", Description: "d", Category: categoryID(1),
		Price:      moneyOf(t, 40000),
		Sourcing:   SourcingRecord{Mode: ModeToOrder, Main: &main, Backup: &backup},
		Attributes: []shared.AttributeValueID{attrValueID(1)},
		Options:    []OptionInput{validOption(t, 1)},
		Photos:     []Photo{photo}, Published: true, Version: 7,
	}
	i, err := RestoreItem(rec)
	if err != nil {
		t.Fatal(err)
	}
	if i.Version() != 7 || !i.Published() || i.Archived() || len(i.Photos()) != 1 || len(i.Options()) != 1 {
		t.Errorf("restored: v%d published %v archived %v photos %d", i.Version(), i.Published(), i.Archived(), len(i.Photos()))
	}
	if i.Cost() == nil || i.Cost().Cents() != 25000 || i.ReadyDays() != 15 {
		t.Errorf("restored cost %v ready %d", i.Cost(), i.ReadyDays())
	}
	if b, ok := i.BackupSupplier(); !ok || b.Cost() != nil {
		t.Errorf("restored backup = %+v, %v", b, ok)
	}

	h, err := RestoreItem(ItemRecord{
		Tenant: tenantID, ID: itemID(2), Name: "Bolo", Price: moneyOf(t, 100),
		Sourcing: SourcingRecord{Mode: ModeInHouse, ProductionDays: 2, ProductionCost: costPtr(t, 40)},
		Archived: true, Version: 1,
	})
	if err != nil || !h.Archived() || h.ProductionCost().Cents() != 40 {
		t.Errorf("restored in_house = %+v, %v", h, err)
	}
}

func TestRestoreItem_RefusesABrokenRecord(t *testing.T) {
	t.Parallel()

	main, err := supplier.NewSupplierItem(supplierID(1), supplier.RoleMain, 15, nil)
	if err != nil {
		t.Fatal(err)
	}
	asBackup, err := supplier.NewSupplierItem(supplierID(1), supplier.RoleBackup, 15, nil)
	if err != nil {
		t.Fatal(err)
	}
	good := func() ItemRecord {
		return ItemRecord{
			Tenant: tenantID, ID: itemID(1), Name: "Banco", Price: moneyOf(t, 40000),
			Sourcing: SourcingRecord{Mode: ModeToOrder, Main: &main}, Version: 1,
		}
	}
	tests := map[string]func(r *ItemRecord){
		"version 0":             func(r *ItemRecord) { r.Version = 0 },
		"untrimmed name":        func(r *ItemRecord) { r.Name = " Banco" },
		"untrimmed description": func(r *ItemRecord) { r.Description = "d " },
		"archived and published": func(r *ItemRecord) {
			r.Archived, r.Published = true, true
		},
		"blank name":   func(r *ItemRecord) { r.Name = "" },
		"zero tenant":  func(r *ItemRecord) { r.Tenant = shared.TenantID{} },
		"unknown mode": func(r *ItemRecord) { r.Sourcing.Mode = "x" },
		"main in the wrong role": func(r *ItemRecord) {
			r.Sourcing.Main = &asBackup
		},
		"too many photos": func(r *ItemRecord) {
			for n := byte(1); n <= 9; n++ {
				r.Photos = append(r.Photos, newPhoto(t, n))
			}
		},
		"the same photo twice": func(r *ItemRecord) { r.Photos = []Photo{newPhoto(t, 1), newPhoto(t, 1)} },
		"an empty photo":       func(r *ItemRecord) { r.Photos = []Photo{{}} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := good()
			mutate(&r)
			_, err := RestoreItem(r)
			if !errors.Is(err, ErrInvalidItem) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalidItem and not a 400", err)
			}
		})
	}
	if _, err := RestoreItem(good()); err != nil {
		t.Errorf("the base record must be valid: %v", err)
	}
}
