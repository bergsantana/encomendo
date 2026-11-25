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

func TestMergeValidation_JoinsEveryField(t *testing.T) {
	t.Parallel()

	name := NewValidationError(FieldError{Field: "name", Message: "Obrigatório."})
	email := fmt.Errorf("parse: %w", NewValidationError(FieldError{Field: "email", Message: "Inválido."}))

	err := MergeValidation(nil, name, nil, email)

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("MergeValidation = %v, want a ValidationError", err)
	}
	if len(ve.Fields) != 2 || ve.Fields[0].Field != "name" || ve.Fields[1].Field != "email" {
		t.Errorf("Fields = %+v, want name then email", ve.Fields)
	}
}

func TestMergeValidation_NilWhenEveryErrorIsNil(t *testing.T) {
	t.Parallel()

	if err := MergeValidation(); err != nil {
		t.Errorf("MergeValidation() = %v", err)
	}
	if err := MergeValidation(nil, nil); err != nil {
		t.Errorf("MergeValidation(nil, nil) = %v", err)
	}
}

func TestMergeValidation_ReturnsOtherErrorsUnchanged(t *testing.T) {
	t.Parallel()

	// An error that is not about the input wins: it is a bug or an outage,
	// and must not turn into a 400.
	other := errors.New("hasher down")
	err := MergeValidation(NewValidationError(FieldError{Field: "name"}), other)
	if err != other { //nolint:errorlint // the very same error must come back, not a wrapper
		t.Errorf("MergeValidation = %v, want the non-validation error itself", err)
	}
}
