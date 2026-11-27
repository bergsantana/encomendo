package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// scrambler returns the Kit Scrambler fixture (R$ 700,00 for R$ 900,00 of
// items) in the given swap mode.
func scrambler(t *testing.T, mode SwapMode) *Kit {
	t.Helper()
	in := validKitInput(t)
	in.SwapMode = mode
	return newKit(t, 1, in, validLookup(t))
}

func swapOf(item, with byte) Swap { return Swap{Item: itemID(item), With: itemID(with)} }

func TestKit_PriceWithSwaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		swaps []Swap
		keep  int64 // cents with SwapKeepDiscount
		lose  int64 // cents with SwapLoseDiscount
	}{
		// Kit price 70000 for seat 40000 + exhaust 30000 + 2 × handlebar 10000.
		{"no swaps is the kit price in both modes", nil, 70000, 70000},
		// keep: 70000 − 40000 + 25000. lose: 25000 + 30000 + 20000.
		{"cheaper seat", []Swap{swapOf(seat, cheapSeat)}, 55000, 75000},
		// keep: 70000 − 40000 + 55000. lose: 55000 + 30000 + 20000.
		{"dearer seat", []Swap{swapOf(seat, richSeat)}, 85000, 105000},
		// A quantity of 2: keep: 70000 − 2 × 10000 + 2 × 15000. lose: 40000 + 30000 + 2 × 15000.
		{"handlebars swapped in the same quantity", []Swap{swapOf(handlebar, highBar)}, 80000, 100000},
		// keep: 70000 − 40000 + 55000 − 20000 + 30000. lose: 55000 + 30000 + 30000.
		{"two swaps", []Swap{swapOf(seat, richSeat), swapOf(handlebar, highBar)}, 95000, 115000},
		{"two swaps in the other order", []Swap{swapOf(handlebar, highBar), swapOf(seat, richSeat)}, 95000, 115000},
	}
	lookup := validLookup(t)
	for _, tt := range tests {
		for mode, want := range map[SwapMode]int64{SwapKeepDiscount: tt.keep, SwapLoseDiscount: tt.lose} {
			t.Run(tt.name+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				got, err := scrambler(t, mode).PriceWithSwaps(lookup, tt.swaps)
				if err != nil {
					t.Fatal(err)
				}
				if got.Cents() != want || got.Currency() != "BRL" {
					t.Errorf("PriceWithSwaps = %v, want %d", got, want)
				}
			})
		}
	}
}

func TestKit_PriceWithSwaps_KeepKeepsTheDiscountLoseLosesIt(t *testing.T) {
	t.Parallel()

	// Swapping a seat for one of the same price must not move the price when
	// the discount is kept, and must cost the full single-item sum when lost.
	lookup := validLookup(t)
	lookup[itemID(cheapSeat)] = facts(t, 40000, 5, 15000)
	swaps := []Swap{swapOf(seat, cheapSeat)}

	keep, err := scrambler(t, SwapKeepDiscount).PriceWithSwaps(lookup, swaps)
	if err != nil || keep.Cents() != 70000 {
		t.Errorf("keep = %v, %v, want the kit price 70000", keep, err)
	}
	lose, err := scrambler(t, SwapLoseDiscount).PriceWithSwaps(lookup, swaps)
	if err != nil || lose.Cents() != 90000 {
		t.Errorf("lose = %v, %v, want the single-item sum 90000", lose, err)
	}
}

func TestKit_PriceWithSwaps_Refusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		swaps []Swap
		want  error
	}{
		{"an alternative the item does not list", []Swap{swapOf(seat, highBar)}, ErrSwapNotListed},
		{"an item that is not in the kit", []Swap{swapOf(cheapSeat, richSeat)}, ErrSwapNotInKit},
		{"an unknown item", []Swap{{Item: itemID(99), With: itemID(highBar)}}, ErrSwapNotInKit},
		{"a non-swappable item", []Swap{swapOf(exhaust, cheapSeat)}, ErrSwapNotAllowed},
		{"the same item swapped twice", []Swap{swapOf(seat, cheapSeat), swapOf(seat, richSeat)}, ErrSwapRepeated},
		{"the same swap twice", []Swap{swapOf(seat, cheapSeat), swapOf(seat, cheapSeat)}, ErrSwapRepeated},
		{"an alternative of another item", []Swap{swapOf(handlebar, cheapSeat)}, ErrSwapNotListed},
		{"a swap to itself", []Swap{swapOf(seat, seat)}, ErrSwapNotListed},
		{"a valid swap then an invalid one", []Swap{swapOf(seat, cheapSeat), swapOf(exhaust, highBar)}, ErrSwapNotAllowed},
	}
	lookup := validLookup(t)
	for _, tt := range tests {
		for _, mode := range []SwapMode{SwapKeepDiscount, SwapLoseDiscount} {
			t.Run(tt.name+"/"+string(mode), func(t *testing.T) {
				t.Parallel()
				k := scrambler(t, mode)
				_, err := k.PriceWithSwaps(lookup, tt.swaps)
				if !errors.Is(err, tt.want) || !errors.Is(err, shared.ErrInvalid) {
					t.Errorf("PriceWithSwaps error = %v, want %v (400)", err, tt.want)
				}
				if _, err := k.DaysWithSwaps(lookup, tt.swaps, 3, 2); !errors.Is(err, tt.want) {
					t.Errorf("DaysWithSwaps error = %v, want %v", err, tt.want)
				}
			})
		}
	}
}

func TestKit_PriceWithSwaps_ItemProblems(t *testing.T) {
	t.Parallel()

	k := scrambler(t, SwapKeepDiscount)
	t.Run("an alternative archived since the kit was saved", func(t *testing.T) {
		t.Parallel()
		lookup := validLookup(t)
		f := lookup[itemID(richSeat)]
		f.Archived = true
		lookup[itemID(richSeat)] = f
		_, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, richSeat)})
		var ae *ArchivedItemsError
		if !errors.As(err, &ae) || len(ae.Items) != 1 || ae.Items[0] != itemID(richSeat) || !errors.Is(err, ErrItemArchived) {
			t.Errorf("error = %v, want ArchivedItemsError for the alternative", err)
		}
		if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, cheapSeat)}); err != nil {
			t.Errorf("a swap to a live alternative = %v, want none", err)
		}
	})
	t.Run("an alternative missing from the lookup", func(t *testing.T) {
		t.Parallel()
		lookup := validLookup(t)
		delete(lookup, itemID(richSeat))
		if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, richSeat)}); !errors.Is(err, ErrItemMissing) {
			t.Errorf("error = %v, want ErrItemMissing", err)
		}
	})
	t.Run("a removed item missing from the lookup", func(t *testing.T) {
		t.Parallel()
		lookup := validLookup(t)
		delete(lookup, itemID(seat))
		if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, richSeat)}); !errors.Is(err, ErrItemMissing) {
			t.Errorf("keep error = %v, want ErrItemMissing", err)
		}
		lose := scrambler(t, SwapLoseDiscount)
		if _, err := lose.PriceWithSwaps(lookup, []Swap{swapOf(handlebar, highBar)}); !errors.Is(err, ErrItemMissing) {
			t.Errorf("lose error = %v, want ErrItemMissing", err)
		}
	})
	t.Run("an item priced in another currency", func(t *testing.T) {
		t.Parallel()
		lookup := validLookup(t)
		lookup[itemID(richSeat)] = ItemFacts{Price: mustMoney(t, 100, "USD")}
		if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, richSeat)}); !errors.Is(err, shared.ErrCurrencyMismatch) {
			t.Errorf("error = %v, want ErrCurrencyMismatch", err)
		}
	})
}

func TestKit_DaysWithSwaps(t *testing.T) {
	t.Parallel()

	lookup := validLookup(t)
	tests := []struct {
		name  string
		swaps []Swap
		want  KitDays
	}{
		{"no swaps is the kit's days", nil, KitDays{15, 3, 2, 20}},
		// The 15-day leather seat leaves; the exhaust (7 days) is now slowest.
		{"a faster seat makes the exhaust the slowest", []Swap{swapOf(seat, cheapSeat)}, KitDays{7, 3, 2, 12}},
		{"a slower seat makes it the slowest", []Swap{swapOf(seat, richSeat)}, KitDays{20, 3, 2, 25}},
		{"a swap that does not change the slowest", []Swap{swapOf(handlebar, highBar)}, KitDays{15, 3, 2, 20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := scrambler(t, SwapKeepDiscount).DaysWithSwaps(lookup, tt.swaps, 3, 2)
			if err != nil || got != tt.want {
				t.Errorf("DaysWithSwaps = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}

	in := validKitInput(t)
	in.AssemblyDays, in.SafetyMarginDays = intPtr(1), intPtr(0)
	got, err := newKit(t, 2, in, lookup).DaysWithSwaps(lookup, []Swap{swapOf(seat, richSeat)}, 3, 2)
	if err != nil || got.Total != 21 {
		t.Errorf("DaysWithSwaps with the kit's own overrides = %+v, %v, want 21 (20 + 1 + 0)", got, err)
	}
	if _, err := scrambler(t, SwapKeepDiscount).DaysWithSwaps(lookup, nil, -1, 0); !errors.Is(err, schedule.ErrNegativeDays) {
		t.Errorf("negative default = %v, want ErrNegativeDays", err)
	}
}

func TestKit_Swaps_DoNotChangeTheKit(t *testing.T) {
	t.Parallel()

	k := scrambler(t, SwapKeepDiscount)
	lookup := validLookup(t)
	if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, richSeat)}); err != nil {
		t.Fatal(err)
	}
	if got := k.Items()[0].Item; got != itemID(seat) || k.Price().Cents() != 70000 {
		t.Errorf("a swap changed the kit: first item %v price %v", got, k.Price())
	}
}

func TestKit_PriceWithSwaps_KeepNeverLeavesAPriceOfZeroOrLess(t *testing.T) {
	t.Parallel()

	// A kit of R$ 50,00 for R$ 900,00 of items: the owner's discount is so
	// large that swapping the R$ 400,00 seat for the R$ 250,00 one would
	// leave 5000 − 40000 + 25000 = −10000.
	lookup := validLookup(t)
	tests := []struct {
		name  string
		price int64
		swaps []Swap
		want  int64 // cents; 0 when the swap must be refused
	}{
		{"negative result", 5000, []Swap{swapOf(seat, cheapSeat)}, 0},
		{"exactly zero", 15000, []Swap{swapOf(seat, cheapSeat)}, 0},
		{"one cent", 15001, []Swap{swapOf(seat, cheapSeat)}, 1},
		{"a swap that raises the price", 5000, []Swap{swapOf(seat, richSeat)}, 20000},
		{"no swap is the kit price, however low", 5000, nil, 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validKitInput(t)
			in.Price = moneyOf(t, tt.price)
			got, err := newKit(t, 1, in, lookup).PriceWithSwaps(lookup, tt.swaps)
			if tt.want == 0 {
				if !errors.Is(err, ErrSwapPriceNotPositive) || !errors.Is(err, shared.ErrConflict) {
					t.Errorf("PriceWithSwaps = %v, %v, want ErrSwapPriceNotPositive (409)", got, err)
				}
				return
			}
			if err != nil || got.Cents() != tt.want {
				t.Errorf("PriceWithSwaps = %v, %v, want %d", got, err, tt.want)
			}
		})
	}
}

func TestKit_PriceWithSwaps_LoseChecksTheKitsCurrency(t *testing.T) {
	t.Parallel()

	usd := ItemLookup{}
	for id, f := range validLookup(t) {
		usd[id] = ItemFacts{Price: mustMoney(t, f.Price.Cents(), "USD"), ReadyDays: f.ReadyDays}
	}
	k := scrambler(t, SwapLoseDiscount)
	if _, err := k.PriceWithSwaps(usd, []Swap{swapOf(seat, cheapSeat)}); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Errorf("lose with every item in USD = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := k.SingleItemsTotal(usd); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Errorf("SingleItemsTotal with every item in USD = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := scrambler(t, SwapKeepDiscount).PriceWithSwaps(usd, []Swap{swapOf(seat, cheapSeat)}); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Errorf("keep with every item in USD = %v, want ErrCurrencyMismatch", err)
	}
}

func TestKit_PriceWithSwaps_RefusesAnItemTheKitAlreadyHolds(t *testing.T) {
	t.Parallel()

	in := validKitInput(t)
	in.Items[0].Alternatives = []shared.ItemID{itemID(cheapSeat), itemID(exhaust)} // exhaust is another member
	lookup := validLookup(t)
	for _, mode := range []SwapMode{SwapKeepDiscount, SwapLoseDiscount} {
		in.SwapMode = mode
		k := newKit(t, 1, in, lookup)
		swaps := []Swap{swapOf(seat, exhaust)}
		if _, err := k.PriceWithSwaps(lookup, swaps); !errors.Is(err, ErrSwapNotAllowed) {
			t.Errorf("%s: swap to a member = %v, want ErrSwapNotAllowed", mode, err)
		}
		if _, err := k.DaysWithSwaps(lookup, swaps, 3, 2); !errors.Is(err, ErrSwapNotAllowed) {
			t.Errorf("%s: days with a swap to a member = %v, want ErrSwapNotAllowed", mode, err)
		}
		if _, err := k.PriceWithSwaps(lookup, []Swap{swapOf(seat, cheapSeat)}); err != nil {
			t.Errorf("%s: a swap to a non-member = %v, want none", mode, err)
		}
	}
}
