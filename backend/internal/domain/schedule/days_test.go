package schedule

import (
	"errors"
	"testing"
)

func TestDays_KitScramblerCB500X(t *testing.T) {
	t.Parallel()

	// docs/product/design-document.md, Deadline formula: the leather seat
	// takes 15 days, the slowest part; 15 + 3 + 2 = 20, and the arrival
	// limit is 20 - 3 = 17.
	got, err := Days([]int{15, 7, 3}, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != 20 {
		t.Errorf("Days([15 7 3], 3, 2) = %d, want 20", got)
	}
}

func TestDays(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ready    []int
		assembly int
		margin   int
		want     int
	}{
		{"slowest is first", []int{15, 7, 3}, 3, 2, 20},
		{"slowest is last", []int{3, 7, 15}, 3, 2, 20},
		{"one item", []int{10}, 3, 2, 15},
		{"empty kit is assembly and margin", nil, 3, 2, 5},
		{"empty, non-nil slice", []int{}, 3, 2, 5},
		{"everything zero", []int{0, 0}, 0, 0, 0},
		{"items ready at once", []int{0}, 4, 1, 5},
		{"ties", []int{7, 7}, 1, 1, 9},
		{"the contract's maximum", []int{365}, 365, 365, 1095},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Days(tt.ready, tt.assembly, tt.margin)
			if err != nil || got != tt.want {
				t.Errorf("Days(%v, %d, %d) = %d, %v; want %d", tt.ready, tt.assembly, tt.margin, got, err, tt.want)
			}
		})
	}
}

func TestDays_RejectsNegativeInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ready    []int
		assembly int
		margin   int
	}{
		{"negative ready days", []int{5, -1}, 3, 2},
		{"negative assembly", []int{5}, -1, 2},
		{"negative margin", []int{5}, 3, -1},
		{"negative with an empty kit", nil, -1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Days(tt.ready, tt.assembly, tt.margin)
			if got != 0 || !errors.Is(err, ErrNegativeDays) {
				t.Errorf("Days(%v, %d, %d) = %d, %v; want 0 and ErrNegativeDays", tt.ready, tt.assembly, tt.margin, got, err)
			}
		})
	}
}

func TestDays_DoesNotChangeItsInput(t *testing.T) {
	t.Parallel()

	ready := []int{15, 7, 3}
	if _, err := Days(ready, 3, 2); err != nil {
		t.Fatal(err)
	}
	if ready[0] != 15 || ready[1] != 7 || ready[2] != 3 {
		t.Errorf("ready = %v, want [15 7 3]", ready)
	}
}

func FuzzDays(f *testing.F) {
	f.Add(15, 7, 3, 3, 2)
	f.Add(0, 0, 0, 0, 0)
	f.Add(-1, 0, 0, 0, 0)
	f.Fuzz(func(t *testing.T, a, b, c, assembly, margin int) {
		const bound = 1 << 30 // keeps the sum from overflowing; real values are 0..365
		for _, v := range []int{a, b, c, assembly, margin} {
			if v > bound || v < -bound {
				t.Skip()
			}
		}
		got, err := Days([]int{a, b, c}, assembly, margin)
		if a < 0 || b < 0 || c < 0 || assembly < 0 || margin < 0 {
			if err == nil || got != 0 {
				t.Fatalf("negative input accepted: %d, %v", got, err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		// The result is at least as long as any single item, and exactly
		// the slowest one plus the fixed days.
		if want := max(a, b, c) + assembly + margin; got != want {
			t.Fatalf("Days = %d, want %d", got, want)
		}
	})
}
