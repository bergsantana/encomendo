package order

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ChosenOption is a client's choice for one of an item's options, as the
// option and choice were when the order was made.
type ChosenOption struct {
	Option     shared.OptionID
	Choice     shared.ChoiceID
	Name       string // the option's name, such as "Cor da costura"
	Label      string // the choice's label, such as "Vermelha"
	ExtraPrice shared.Money
	ExtraDays  int
}

// ItemSnapshot is what the client may see of an item at order time. It has
// no supplier and no cost: those are in Sourcing, a separate type, so the
// repository writes them to a staff-only table and no client view can carry
// them by accident (ADR 0030; docs/architecture.md section 2, rule 8).
type ItemSnapshot struct {
	CatalogItem  shared.ItemID
	Name         string
	SourcingMode catalog.SourcingMode
	UnitPrice    shared.Money // the single-item price, options excluded
	Quantity     int          // 1..99
	Options      []ChosenOption
	LeadDays     int // ready days at order time: the main supplier's lead time or the production days
}

// extraDays returns the days the chosen options add to the item's ready
// date (ADR 0024).
func (s ItemSnapshot) extraDays() int {
	days := 0
	for _, o := range s.Options {
		days += o.ExtraDays
	}
	return days
}

// Sourcing is the staff-only part of an item's snapshot: where it comes
// from and what it cost at order time. Only the owner sees the cost
// (identity.SeesCosts, ADR 0025).
type Sourcing struct {
	Supplier     shared.SupplierID // zero for an in-house item
	SupplierName string
	Cost         *shared.Money // per unit; nil when unknown
	Backup       *BackupSource // nil when the item has none, or after switching to it
}

// BackupSource is an item's backup supplier at order time, the first way to
// fix a delay ("Usar o fornecedor reserva").
type BackupSource struct {
	Supplier shared.SupplierID
	Name     string
	LeadDays int
	Cost     *shared.Money // per unit; nil when unknown
}

// KitSnapshot is the kit a kit line was sold as: its name and the swaps the
// client made, with the item names as they were.
type KitSnapshot struct {
	Kit   shared.KitID
	Name  string
	Swaps []SwapSnapshot
}

// SwapSnapshot is one swap of a kit line: Removed was replaced by Added.
type SwapSnapshot struct {
	Removed, Added         shared.ItemID
	RemovedName, AddedName string
}
