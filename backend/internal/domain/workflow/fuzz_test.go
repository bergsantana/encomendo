package workflow

import (
	"errors"
	"testing"
)

// splitmix returns the next value of the SplitMix64 sequence. The fuzzer
// supplies the seed, so a failing input replays exactly.
func splitmix(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// FuzzValidate_ShuffledPresets shuffles a preset's statuses. Every preset
// holds the four required phases and unique keys, so a shuffle is valid
// exactly when no phase comes before a lower one: a shuffle that puts a
// phase back must fail, and one that keeps the order must pass.
func FuzzValidate_ShuffledPresets(f *testing.F) {
	for i := range uint8(4) {
		f.Add(i, uint64(i)*7919, uint8(0))
		f.Add(i, uint64(i)+1, uint8(3))
	}
	f.Fuzz(func(t *testing.T, which uint8, seed uint64, swaps uint8) {
		presets := seededPresets()
		statuses := presets[int(which)%len(presets)].statuses
		state := seed
		for range swaps {
			i := int(splitmix(&state) % uint64(len(statuses)))
			j := int(splitmix(&state) % uint64(len(statuses)))
			statuses[i], statuses[j] = statuses[j], statuses[i]
		}
		sorted := true
		for i := 1; i < len(statuses); i++ {
			if statuses[i].Phase.Rank() < statuses[i-1].Phase.Rank() {
				sorted = false
			}
		}
		err := Validate(statuses)
		switch {
		case sorted && err != nil:
			t.Fatalf("phases in order but Validate = %v", err)
		case !sorted && !errors.Is(err, ErrInvalidWorkflow):
			t.Fatalf("a phase goes back but Validate = %v", err)
		}
	})
}
