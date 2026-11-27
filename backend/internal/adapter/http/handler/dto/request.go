package dto

import (
	"bytes"
	"encoding/json"
	"fmt"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Optional is a request field that tells three states apart: the key is
// absent (Set is false), the key is null (Set and Null), or it has a
// value. A plain pointer cannot, because encoding/json leaves both an
// absent key and null as nil. Its UnmarshalJSON runs only when the key is
// present, which is how an absent key is told from null (ADR 0025).
type Optional[T any] struct {
	// Set is true when the key was in the body, with a value or null.
	Set bool
	// Null is true when the key was sent as null.
	Null bool
	// Value is the value sent; the zero value when absent or null.
	Value T
}

// UnmarshalJSON records that the key was sent. It decodes with the same
// strictness as the request: an unknown field inside a value is an error,
// because a nested decoder would otherwise not inherit
// DisallowUnknownFields.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o.Value); err != nil {
		return err
	}
	return nil
}

// CostField turns a cost key of a request into the use case's tri-state:
// absent keeps the stored cost, null clears it, a value sets it. Any key
// that was sent, even null, is refused for a role that does not see costs
// (ADR 0025), which the use case decides.
func CostField(o Optional[int64]) appcatalog.CostField {
	switch {
	case !o.Set:
		return appcatalog.KeepCost()
	case o.Null:
		return appcatalog.ClearCost()
	default:
		return appcatalog.SetCost(o.Value)
	}
}

// fields collects the problems of a request body, so one answer names every
// missing or malformed field (400 invalid, with errors).
type fields struct {
	issues []shared.FieldError
}

func (f *fields) add(field, message string) {
	f.issues = append(f.issues, shared.FieldError{Field: field, Message: message})
}

// err returns a ValidationError listing every issue, or nil when none.
func (f *fields) err() error {
	if len(f.issues) == 0 {
		return nil
	}
	return shared.NewValidationError(f.issues...)
}

const (
	msgRequired = "Obrigatório."
	msgBadID    = "Identificador inválido."
)

// need returns *v, or records that field is missing.
func need[T any](f *fields, v *T, field string) T {
	if v == nil {
		f.add(field, msgRequired)
		var zero T
		return zero
	}
	return *v
}

// needList returns v, or records that field is missing when v is nil: an
// absent key and null both decode to a nil slice, and an empty array is
// not nil.
func needList[T any](f *fields, v []T, field string) []T {
	if v == nil {
		f.add(field, msgRequired)
	}
	return v
}

// needKey records that field is missing when its key is absent. null is
// accepted: the field is required but nullable.
func needKey[T any](f *fields, o Optional[T], field string) {
	if !o.Set {
		f.add(field, msgRequired)
	}
}

// nullable returns the value of o when one was sent, with whether it was.
// Absent and null both mean "no value".
func nullable[T any](o Optional[T]) (T, bool) {
	return o.Value, o.Set && !o.Null
}

// parseID parses an id in field, recording an issue when it is not one.
func parseID[T ~[16]byte](f *fields, raw, field string) T {
	id, err := shared.ParseID(raw)
	if err != nil {
		f.add(field, msgBadID)
		return T{}
	}
	return T(id)
}

// needID parses a required id, recording a missing key or a malformed id.
func needID[T ~[16]byte](f *fields, raw *string, field string) T {
	if raw == nil {
		f.add(field, msgRequired)
		return T{}
	}
	return parseID[T](f, *raw, field)
}

// parseIDs parses a list of ids, naming the failing element.
func parseIDs[T ~[16]byte](f *fields, raw []string, field string) []T {
	out := make([]T, 0, len(raw))
	for i, s := range raw {
		out = append(out, parseID[T](f, s, fmt.Sprintf("%s[%d]", field, i)))
	}
	return out
}

// Owned is a response field only the owner sees (ADR 0025): a nil *Owned is
// left out of the JSON altogether, and a non-nil one is written as the value
// or as null when nothing is recorded. A plain pointer with omitempty cannot
// tell "absent for this role" from "null for the owner".
type Owned[T any] struct{ value *T }

// Own returns the owner's field holding v; a nil v is written as null.
func Own[T any](v *T) *Owned[T] { return &Owned[T]{value: v} }

// MarshalJSON writes the value, or null when none is recorded.
func (o Owned[T]) MarshalJSON() ([]byte, error) {
	if o.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.value)
}
