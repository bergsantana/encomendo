package order

import (
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
)

// ItemStatus is where one order item stands. Each sourcing mode has its own
// lifecycle (docs/architecture.md section 4.2, Item lifecycle; ADR 0030).
// The values are part of the API contract.
type ItemStatus string

// The item statuses.
const (
	ItemToBuy          ItemStatus = "to_buy"           // A comprar
	ItemSentToSupplier ItemStatus = "sent_to_supplier" // Enviada ao fornecedor
	ItemInTransit      ItemStatus = "in_transit"       // Em trânsito
	ItemReceived       ItemStatus = "received"         // Recebida
	ItemPlanned        ItemStatus = "planned"          // Planejado
	ItemInProduction   ItemStatus = "in_production"    // Em produção
	ItemReady          ItemStatus = "ready"            // Pronto
	ItemInstalled      ItemStatus = "installed"        // Instalada
)

// Check is the result of checking an item when it is received or made:
// Tudo certo or Com defeito.
type Check string

// The checks.
const (
	CheckOK        Check = "ok"
	CheckDefective Check = "defective"
)

// Valid reports whether c is a known check.
func (c Check) Valid() bool { return c == CheckOK || c == CheckDefective }

// bought is the lifecycle of to_order items and, until Estoque lands in
// milestone 6, of stock items (ADR 0030). Each status lists where it may go:
// forward one step, straight to received (bought at the counter), or one
// step back to undo a mistake. Nothing leaves installed.
var bought = map[ItemStatus][]ItemStatus{
	ItemToBuy:          {ItemSentToSupplier, ItemReceived},
	ItemSentToSupplier: {ItemInTransit, ItemReceived, ItemToBuy},
	ItemInTransit:      {ItemReceived, ItemSentToSupplier},
	ItemReceived:       {ItemInstalled},
	ItemInstalled:      nil,
}

// made is the lifecycle of in-house items.
var made = map[ItemStatus][]ItemStatus{
	ItemPlanned:      {ItemInProduction},
	ItemInProduction: {ItemReady, ItemPlanned},
	ItemReady:        {ItemInstalled},
	ItemInstalled:    nil,
}

func lifecycle(m catalog.SourcingMode) map[ItemStatus][]ItemStatus {
	if m == catalog.ModeInHouse {
		return made
	}
	return bought
}

// InitialStatus returns where a new item of mode m starts: to_buy for
// to_order and stock items, planned for in-house ones.
func InitialStatus(m catalog.SourcingMode) ItemStatus {
	if m == catalog.ModeInHouse {
		return ItemPlanned
	}
	return ItemToBuy
}

// needsCheck reports whether moving to s records a check: an item is
// checked when it is received or made.
func needsCheck(s ItemStatus) bool { return s == ItemReceived || s == ItemReady }

// CanMoveItem reports whether an item of mode m may move from one status to
// another, with check. It returns ErrInvalidItemMove when the lifecycle has
// no such step, when a move to received or ready comes without a valid
// check, or when any other move comes with one.
func CanMoveItem(m catalog.SourcingMode, from, to ItemStatus, check *Check) error {
	if !slices.Contains(lifecycle(m)[from], to) {
		return ErrInvalidItemMove
	}
	if needsCheck(to) != (check != nil) {
		return ErrInvalidItemMove
	}
	if check != nil && !check.Valid() {
		return ErrInvalidItemMove
	}
	return nil
}

// NextItemMoves returns the statuses an item of mode m may move to from
// from, in lifecycle order, as a new slice.
func NextItemMoves(m catalog.SourcingMode, from ItemStatus) []ItemStatus {
	return slices.Clone(lifecycle(m)[from])
}

// landing returns where a move to to with check leaves the item: a
// defective item goes back to be bought again or made again
// (docs/architecture.md section 4.2).
func landing(m catalog.SourcingMode, to ItemStatus, check *Check) ItemStatus {
	if check == nil || *check != CheckDefective {
		return to
	}
	if m == catalog.ModeInHouse {
		return ItemInProduction
	}
	return ItemToBuy
}

// arrived reports whether an item in status s is ready to be assembled:
// received or made with a good check, or already installed. A defective
// check never lands on received or ready, so the status alone says it.
func arrived(s ItemStatus) bool {
	return s == ItemReceived || s == ItemReady || s == ItemInstalled
}

// validItemStatus reports whether s belongs to mode m's lifecycle.
func validItemStatus(m catalog.SourcingMode, s ItemStatus) bool {
	_, ok := lifecycle(m)[s]
	return ok
}
