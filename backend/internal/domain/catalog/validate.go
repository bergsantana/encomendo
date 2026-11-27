package catalog

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// maxAmountCents is the largest price, cost or extra price: the contract's
// Cents maximum (R$ 1.000.000.000,00). Bounding every amount keeps margin
// and kit arithmetic far from int64 overflow.
const maxAmountCents = 100_000_000_000

const (
	maxNameLength        = 120
	maxShortNameLength   = 80
	maxDescriptionLength = 2000
	maxProductionDays    = 365
	maxDays              = 365
	maxAttributeValues   = 200
)

// textRule describes one single-line text field: its contract name, its
// maximum length in characters and the messages for each way to break it.
type textRule struct {
	field    string
	max      int
	required string
	tooLong  string
	control  string
}

// check reports a ValidationError on the rule's field unless s, already
// trimmed, has 1 to max characters, valid UTF-8 and no control character. A
// text that breaks the last two gets its own message: shortening it would
// not help.
func (r textRule) check(s string) error {
	switch {
	case s == "":
		return fieldError(r.field, r.required)
	case !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl):
		return fieldError(r.field, r.control)
	case utf8.RuneCountInString(s) > r.max:
		return fieldError(r.field, r.tooLong)
	}
	return nil
}

var (
	itemNameRule  = textRule{fieldName, maxNameLength, msgItemNameRequired, msgNameTooLong, msgNameControl}
	kitNameRule   = textRule{fieldName, maxNameLength, msgKitNameRequired, msgNameTooLong, msgNameControl}
	categoryRule  = textRule{fieldName, maxNameLength, msgCategoryNameRequired, msgNameTooLong, msgNameControl}
	attributeRule = textRule{fieldName, maxNameLength, msgAttributeNameMissing, msgNameTooLong, msgNameControl}
	optionRule    = textRule{fieldName, maxShortNameLength, msgOptionNameRequired, msgShortNameTooLong, msgNameControl}
	choiceRule    = textRule{fieldLabel, maxShortNameLength, msgChoiceRequired, msgChoiceTooLong, msgChoiceControl}
	valueRule     = textRule{fieldLabel, maxShortNameLength, msgValueRequired, msgValueTooLong, msgValueControl}
)

// checkDescription reports a ValidationError on "description" when s, already
// trimmed, is not valid UTF-8 or has more than 2000 characters. Line breaks
// are allowed.
func checkDescription(s string) error {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxDescriptionLength {
		return fieldError(fieldDescription, msgDescriptionTooLong)
	}
	return nil
}

func fieldError(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// prefixFields returns err with prefix and a dot put before the field of
// every FieldError, so an error of a nested value names the path the client
// sent. Errors that are not a ValidationError come back unchanged.
func prefixFields(err error, prefix string) error {
	ve, ok := err.(*shared.ValidationError) //nolint:errorlint // only a direct ValidationError is rewritten; anything wrapped is passed through as it is
	if !ok {
		return err
	}
	out := make([]shared.FieldError, len(ve.Fields))
	for i, f := range ve.Fields {
		out[i] = shared.FieldError{Field: prefix + "." + f.Field, Message: f.Message}
	}
	return shared.NewValidationError(out...)
}

// at writes the path of element i of a list field, such as "options[2]".
func at(field string, i int) string { return fmt.Sprintf("%s[%d]", field, i) }

// checkAmount reports a ValidationError on field when m is above the largest
// amount the catalog accepts.
func checkAmount(field string, m shared.Money) error {
	if m.Cents() > maxAmountCents {
		return fieldError(field, msgAmountTooHigh)
	}
	return nil
}

// hasDuplicate reports whether ids holds the same value twice.
func hasDuplicate[T comparable](ids []T) bool {
	seen := make(map[T]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}

// checkAttributeValues reports a ValidationError on "attributeValueIds" when
// there are more than 200 values or one is repeated, and ErrInvalidItem or
// ErrInvalidKit (given as invalid) for a zero id.
func checkAttributeValues(ids []shared.AttributeValueID, invalid error) error {
	for _, id := range ids {
		if id.IsZero() {
			return fmt.Errorf("zero attribute value id: %w", invalid)
		}
	}
	switch {
	case len(ids) > maxAttributeValues:
		return fieldError(fieldAttributeValues, msgTooManyValues)
	case hasDuplicate(ids):
		return fieldError(fieldAttributeValues, msgValueRepeated)
	}
	return nil
}

// sameCurrency reports whether m is in currency; an empty Money (no currency)
// is never in any.
func sameCurrency(m shared.Money, currency string) bool { return m.Currency() == currency }

func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append([]T(nil), s...)
}
