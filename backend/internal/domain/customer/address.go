package customer

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Limits of an address, shared with the contract (docs/api/openapi.yaml,
// AddressInput).
const (
	maxLabelLength      = 40
	maxStreetLength     = 200
	maxNumberLength     = 20
	maxComplementLength = 100
	maxDistrictLength   = 100
	maxCityLength       = 100
	cepLength           = 8
)

// ufs holds the 27 Brazilian state codes the API accepts (docs/api/openapi.yaml, UF).
var ufs = map[string]struct{}{
	"AC": {}, "AL": {}, "AP": {}, "AM": {}, "BA": {}, "CE": {}, "DF": {}, "ES": {}, "GO": {}, "MA": {},
	"MT": {}, "MS": {}, "MG": {}, "PA": {}, "PB": {}, "PR": {}, "PE": {}, "PI": {}, "RJ": {}, "RN": {},
	"RS": {}, "RO": {}, "RR": {}, "SC": {}, "SP": {}, "SE": {}, "TO": {},
}

// Address is one place a customer takes orders to. Fields are trimmed, and
// the ones a customer cannot leave empty always hold text; Number,
// Complement and District may be empty. CEP is 8 digits and State a UF code.
type Address struct {
	ID         shared.AddressID
	Label      string
	CEP        string
	Street     string
	Number     string
	Complement string
	District   string
	City       string
	State      string
}

// AddressSpec is an address as a person typed it, before it is checked. ID is
// set by the app layer: the one the client sent to keep an address, or a new
// one.
type AddressSpec struct {
	ID         shared.AddressID
	Label      string
	CEP        string
	Street     string
	Number     string
	Complement string
	District   string
	City       string
	State      string
}

// textRule describes one free-text field of an address.
type textRule struct {
	field, value string
	max          int
	required     bool
	msgRequired  string
	msgTooLong   string
}

// checkAddress trims s and reports every invalid field, named
// "addresses[i].field". A zero or repeated id is a bug, not input, and comes
// back as ErrInvalidCustomer.
func checkAddress(i int, s AddressSpec) (Address, error) {
	if s.ID.IsZero() {
		return Address{}, fmt.Errorf("address %d has no id: %w", i, ErrInvalidCustomer)
	}
	a, err := checkFields(s)
	return a, prefixFields(err, at(fieldAddresses, i))
}

// CheckNewAddress trims s and checks it as AddAddress would for a customer
// who already has have addresses, without a customer: the order quote
// validates a "Novo endereço" it never saves. A ValidationError names the
// field bare ("cep", "street"), or "addresses" for the limit of 10. It
// returns ErrInvalidCustomer for a zero id, which is a bug, not input.
func CheckNewAddress(have int, s AddressSpec) (Address, error) {
	if have >= maxAddresses {
		return Address{}, fieldError(fieldAddresses, msgTooManyAddrs)
	}
	if s.ID.IsZero() {
		return Address{}, fmt.Errorf("new address has no id: %w", ErrInvalidCustomer)
	}
	return checkFields(s)
}

// checkFields trims s and reports every invalid field by its bare name.
func checkFields(s AddressSpec) (Address, error) {
	a := Address{
		ID:         s.ID,
		Label:      strings.TrimSpace(s.Label),
		CEP:        strings.TrimSpace(s.CEP),
		Street:     strings.TrimSpace(s.Street),
		Number:     strings.TrimSpace(s.Number),
		Complement: strings.TrimSpace(s.Complement),
		District:   strings.TrimSpace(s.District),
		City:       strings.TrimSpace(s.City),
		State:      strings.TrimSpace(s.State),
	}
	rules := []textRule{
		{fieldLabel, a.Label, maxLabelLength, true, msgLabelRequired, msgLabelTooLong},
		{fieldStreet, a.Street, maxStreetLength, true, msgStreetRequired, msgStreetTooLong},
		{fieldNumber, a.Number, maxNumberLength, false, "", msgNumberTooLong},
		{fieldComplement, a.Complement, maxComplementLength, false, "", msgComplementTooLong},
		{fieldDistrict, a.District, maxDistrictLength, false, "", msgDistrictTooLong},
		{fieldCity, a.City, maxCityLength, true, msgCityRequired, msgCityTooLong},
	}
	var errs []error
	for _, r := range rules {
		errs = append(errs, checkText(r))
	}
	errs = append(errs, checkCEP(a.CEP), checkState(a.State))
	return a, shared.MergeValidation(errs...)
}

// checkText reports a ValidationError on r.field when the value is missing
// (for a required field), too long, or not valid text.
func checkText(r textRule) error {
	switch {
	case r.value == "" && r.required:
		return fieldError(r.field, r.msgRequired)
	case !utf8.ValidString(r.value) || strings.ContainsFunc(r.value, unicode.IsControl):
		return fieldError(r.field, msgAddressTextControl)
	case utf8.RuneCountInString(r.value) > r.max:
		return fieldError(r.field, r.msgTooLong)
	}
	return nil
}

// checkCEP reports a ValidationError on "cep" unless cep is exactly 8 ASCII
// digits, without a hyphen: the contract sends digits only.
func checkCEP(cep string) error {
	if len(cep) != cepLength {
		return fieldError(fieldCEP, msgCEPInvalid)
	}
	for i := range len(cep) {
		if cep[i] < '0' || cep[i] > '9' {
			return fieldError(fieldCEP, msgCEPInvalid)
		}
	}
	return nil
}

// checkState reports a ValidationError on "state" unless state is one of the
// 27 UF codes, in capitals.
func checkState(state string) error {
	if _, ok := ufs[state]; !ok {
		return fieldError(fieldState, msgStateInvalid)
	}
	return nil
}

func fieldError(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// prefixFields returns err with prefix and a dot put before the field of
// every FieldError, so an error of an address names the path the client sent.
// Anything that is not a direct ValidationError, including nil, comes back
// unchanged.
func prefixFields(err error, prefix string) error {
	ve, ok := err.(*shared.ValidationError) //nolint:errorlint // only a direct ValidationError built by MergeValidation is rewritten; anything else passes through as it is
	if !ok {
		return err
	}
	out := make([]shared.FieldError, len(ve.Fields))
	for i, f := range ve.Fields {
		out[i] = shared.FieldError{Field: prefix + "." + f.Field, Message: f.Message}
	}
	return shared.NewValidationError(out...)
}

// at writes the path of element i of a list field, such as "addresses[2]".
func at(field string, i int) string { return fmt.Sprintf("%s[%d]", field, i) }
