package catalog

import (
	"fmt"
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Swap is one change a client makes to a kit: the kit item it removes and
// the listed alternative that takes its place, in the same quantity. A kit
// item cannot be removed without a replacement (ADR 0025).
type Swap struct {
	Item shared.ItemID
	With shared.ItemID
}

// swapped checks swaps against the kit and returns what each one removes and
// adds, plus the kit's lines after all of them. The checks, in order for
// each swap: the item is in the kit (ErrSwapNotInKit), it is not swapped
// twice (ErrSwapRepeated), it is swappable (ErrSwapNotAllowed), and With is
// one of its listed alternatives (ErrSwapNotListed). An alternative that was
// archived since the kit was saved gives an *ArchivedItemsError.
func (k *Kit) swapped(lookup ItemLookup, swaps []Swap) (lines, removed, added []line, err error) {
	lines = k.lines()
	done := make(map[shared.ItemID]struct{}, len(swaps))
	for _, sw := range swaps {
		i := slices.IndexFunc(k.items, func(ki KitItem) bool { return ki.Item == sw.Item })
		if err := k.checkSwap(i, sw, done); err != nil {
			return nil, nil, nil, err
		}
		with, err := lookup.facts(sw.With)
		if err != nil {
			return nil, nil, nil, err
		}
		if with.Archived {
			return nil, nil, nil, &ArchivedItemsError{Items: []shared.ItemID{sw.With}}
		}
		done[sw.Item] = struct{}{}
		removed = append(removed, line{sw.Item, k.items[i].Quantity})
		added = append(added, line{sw.With, k.items[i].Quantity})
		lines[i] = line{sw.With, k.items[i].Quantity}
	}
	return lines, removed, added, nil
}

// checkSwap applies the swap rules that need only the kit; i is the index of
// the swap's item in the kit, or -1.
func (k *Kit) checkSwap(i int, sw Swap, done map[shared.ItemID]struct{}) error {
	if i < 0 {
		return fmt.Errorf("swap of %v in kit %v: %w", sw.Item, k.id, ErrSwapNotInKit)
	}
	if _, twice := done[sw.Item]; twice {
		return fmt.Errorf("swap of %v in kit %v: %w", sw.Item, k.id, ErrSwapRepeated)
	}
	ki := k.items[i]
	if !ki.Swappable {
		return fmt.Errorf("swap of %v in kit %v: %w", sw.Item, k.id, ErrSwapNotAllowed)
	}
	if !slices.Contains(ki.Alternatives, sw.With) {
		return fmt.Errorf("swap of %v for %v in kit %v: %w", sw.Item, sw.With, k.id, ErrSwapNotListed)
	}
	return nil
}

// PriceWithSwaps returns what the client pays for the kit after swaps, using
// single-item prices times quantity (ADR 0023, ADR 0025). With no swaps the
// price is the kit's price, whatever the mode. With swaps it follows the
// kit's SwapMode:
//   - SwapKeepDiscount: kit price − each removed item × quantity + each new
//     item × quantity, so the kit's discount stays;
//   - SwapLoseDiscount: the sum of the resulting items' single-item prices ×
//     quantity.
//
// It returns ErrSwapNotInKit, ErrSwapNotAllowed, ErrSwapNotListed or
// ErrSwapRepeated for a swap that breaks the rules (see Swap), an
// *ArchivedItemsError for an archived alternative, ErrItemMissing when the
// lookup lacks an item, and shared.ErrCurrencyMismatch when an item is priced
// in another currency than the kit.
func (k *Kit) PriceWithSwaps(lookup ItemLookup, swaps []Swap) (shared.Money, error) {
	lines, removed, added, err := k.swapped(lookup, swaps)
	if err != nil {
		return shared.Money{}, err
	}
	if len(swaps) == 0 {
		return k.price, nil
	}
	if k.swapMode == SwapLoseDiscount {
		return singleTotal(lines, lookup)
	}
	return keepPrice(k.price, lookup, removed, added)
}

// keepPrice is the price − removed + added of SwapKeepDiscount.
func keepPrice(price shared.Money, lookup ItemLookup, removed, added []line) (shared.Money, error) {
	out := price
	for _, step := range []struct {
		lines []line
		sign  int
	}{{removed, -1}, {added, 1}} {
		if len(step.lines) == 0 {
			continue
		}
		total, err := singleTotal(step.lines, lookup)
		if err != nil {
			return shared.Money{}, err
		}
		if step.sign < 0 {
			out, err = out.Sub(total)
		} else {
			out, err = out.Add(total)
		}
		if err != nil {
			return shared.Money{}, err
		}
	}
	return out, nil
}

// DaysWithSwaps returns the kit's days after swaps: the slowest of the
// resulting items' ready days + assembly + margin, as Days does with the kit's
// own overrides. Errors are those of PriceWithSwaps and Days.
func (k *Kit) DaysWithSwaps(lookup ItemLookup, swaps []Swap, defaultAssembly, defaultMargin int) (KitDays, error) {
	lines, _, _, err := k.swapped(lookup, swaps)
	if err != nil {
		return KitDays{}, err
	}
	assembly, margin := k.effectiveDays(defaultAssembly, defaultMargin)
	return daysOf(lines, lookup, assembly, margin)
}
