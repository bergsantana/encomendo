package shared

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidationError_MatchesErrInvalid(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("create order: %w", NewValidationError(
		FieldError{Field: "phone", Message: "Informe um WhatsApp válido."},
	))

	if !errors.Is(err, ErrInvalid) {
		t.Fatal("errors.Is(err, ErrInvalid) = false for a wrapped ValidationError")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As did not find the ValidationError")
	}
	if len(ve.Fields) != 1 || ve.Fields[0].Field != "phone" {
		t.Errorf("Fields = %+v", ve.Fields)
	}
}

func TestValidationError_ErrorListsFields(t *testing.T) {
	t.Parallel()

	err := NewValidationError(
		FieldError{Field: "name", Message: "Obrigatório."},
		FieldError{Field: "phone", Message: "Obrigatório."},
	)

	if got, want := err.Error(), "invalid input: name, phone"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSentinels_AreDistinct(t *testing.T) {
	t.Parallel()

	all := []error{ErrNotFound, ErrConflict, ErrForbidden, ErrInvalid, ErrUnauthenticated}
	for i, a := range all {
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v matches %v", a, b)
			}
		}
	}
}

func TestErrVersionConflict_IsAConflict(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("save settings: %w", ErrVersionConflict)
	if !errors.Is(err, ErrVersionConflict) || !errors.Is(err, ErrConflict) {
		t.Errorf("%v should match ErrVersionConflict and ErrConflict", err)
	}
	if errors.Is(ErrConflict, ErrVersionConflict) {
		t.Error("a plain conflict must not read as a version conflict")
	}
}
