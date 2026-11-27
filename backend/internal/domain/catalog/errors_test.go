package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestErrorsMatchSharedSentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{"ErrItemNotFound", ErrItemNotFound, shared.ErrNotFound},
		{"ErrKitNotFound", ErrKitNotFound, shared.ErrNotFound},
		{"ErrCategoryNotFound", ErrCategoryNotFound, shared.ErrNotFound},
		{"ErrAttributeNotFound", ErrAttributeNotFound, shared.ErrNotFound},
		{"ErrPhotoNotFound", ErrPhotoNotFound, shared.ErrNotFound},
		{"ErrItemMissing", ErrItemMissing, shared.ErrNotFound},
		{"ErrNameTaken", ErrNameTaken, shared.ErrConflict},
		{"ErrItemInKit", ErrItemInKit, shared.ErrConflict},
		{"ErrCategoryInUse", ErrCategoryInUse, shared.ErrConflict},
		{"ErrItemArchived", ErrItemArchived, shared.ErrConflict},
		{"ErrPhotoLimit", ErrPhotoLimit, shared.ErrConflict},
		{"ErrPublishArchived", ErrPublishArchived, shared.ErrInvalid},
		{"ErrPhotoInvalid", ErrPhotoInvalid, shared.ErrInvalid},
		{"ErrSwapNotInKit", ErrSwapNotInKit, shared.ErrInvalid},
		{"ErrSwapNotAllowed", ErrSwapNotAllowed, shared.ErrInvalid},
		{"ErrSwapNotListed", ErrSwapNotListed, shared.ErrInvalid},
		{"ErrSwapRepeated", ErrSwapRepeated, shared.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(tt.err, tt.want) {
				t.Errorf("%v does not match %v", tt.err, tt.want)
			}
		})
	}
}

func TestProgrammingErrorsMatchNoSharedSentinel(t *testing.T) {
	t.Parallel()

	for name, err := range map[string]error{
		"ErrInvalidItem": ErrInvalidItem, "ErrInvalidKit": ErrInvalidKit, "ErrInvalidCategory": ErrInvalidCategory,
		"ErrInvalidAttribute": ErrInvalidAttribute, "ErrInvalidOption": ErrInvalidOption,
	} {
		for _, sentinel := range []error{shared.ErrInvalid, shared.ErrConflict, shared.ErrNotFound, shared.ErrForbidden} {
			if errors.Is(err, sentinel) {
				t.Errorf("%s matches %v, but a bug must answer 500", name, sentinel)
			}
		}
	}
}

func TestErrItemInKit_IsAConflictTheAppLayerReturns(t *testing.T) {
	t.Parallel()

	// Only the app layer knows the kits; the domain provides the sentinel.
	if !errors.Is(ErrItemInKit, shared.ErrConflict) || errors.Is(ErrItemInKit, ErrItemArchived) {
		t.Error("ErrItemInKit must be its own 409 code, apart from item_archived")
	}
}
