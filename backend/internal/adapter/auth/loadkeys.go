package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// DevSigningSeed is the fixed signing seed of the local and CI stacks
// (ADR 0019). It is in the repository, so anyone could sign owner tokens
// for every store with it: LoadKeys refuses it outside development and
// test, as a signing key and as a verify-only key.
const DevSigningSeed = "augERHUoTDTq1K69k5GLuA1cZM6mNFE3RPy9bYzpj08="

// LoadKeys builds the key set from the configured seeds: the signing seed
// and the verify-only seeds of a rotation (ADR 0019). A missing or invalid
// seed is an error: there is no fallback key. When allowDevKey is false
// (production), a key with DevSigningSeed's kid is refused wherever it
// appears, compared by kid so a second spelling of the seed cannot slip by.
func LoadKeys(signingSeed string, verifySeeds []string, allowDevKey bool) (KeySet, error) {
	if strings.TrimSpace(signingSeed) == "" {
		return KeySet{}, errors.New("auth: no signing seed (AUTH_SIGNING_SEED); there is no fallback key")
	}
	signing, err := ParseKey(signingSeed)
	if err != nil {
		return KeySet{}, fmt.Errorf("auth: the signing seed: %w", err)
	}
	var verifyOnly []Key
	for i, seed := range verifySeeds {
		k, err := ParseKey(seed)
		if err != nil {
			return KeySet{}, fmt.Errorf("auth: verify-only seed %d: %w", i+1, err)
		}
		verifyOnly = append(verifyOnly, k)
	}
	keys, err := NewKeySet(signing, verifyOnly...)
	if err != nil {
		return KeySet{}, err
	}
	if !allowDevKey {
		dev, err := ParseKey(DevSigningSeed)
		if err != nil {
			return KeySet{}, fmt.Errorf("auth: the development seed: %w", err)
		}
		if slices.Contains(keys.KIDs(), dev.KID()) {
			return KeySet{}, errors.New("auth: the development signing key is configured outside development; set a seed of your own")
		}
	}
	return keys, nil
}
