package catalog

import (
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	maxChoices = 50
	maxOptions = 20
)

// ChoiceInput is one choice of an option as a client of the domain sends it.
// ExtraPrice is in the item's currency.
type ChoiceInput struct {
	ID         shared.ChoiceID
	Label      string
	ExtraPrice shared.Money
	ExtraDays  int
}

// OptionInput is a customization option as a client of the domain sends it.
// The ids are the caller's: it keeps the id of an option or choice that
// already exists and generates one for a new one.
type OptionInput struct {
	ID       shared.OptionID
	Name     string
	Required bool
	Choices  []ChoiceInput
}

// Choice is one pick of a customization option: its label, what it adds to
// the item's price and the days it adds to the item's ready date. It is an
// immutable value.
type Choice struct {
	id         shared.ChoiceID
	label      string
	extraPrice shared.Money
	extraDays  int
}

// ID returns the choice's id.
func (c Choice) ID() shared.ChoiceID { return c.id }

// Label returns the text the client sees, trimmed.
func (c Choice) Label() string { return c.label }

// ExtraPrice returns what the choice adds to the item's price; zero or more.
func (c Choice) ExtraPrice() shared.Money { return c.extraPrice }

// ExtraDays returns the days the choice adds to the item's ready date, 0 to
// 365 (milestone 4's deadline adds them).
func (c Choice) ExtraDays() int { return c.extraDays }

// CustomizationOption is a question the client answers when ordering an item
// (ADR 0024): exactly one choice when it is required, at most one otherwise.
// There are no free-text fields and no rules between options in v1. It is an
// immutable value.
type CustomizationOption struct {
	id       shared.OptionID
	name     string
	required bool
	choices  []Choice
}

// ID returns the option's id.
func (o CustomizationOption) ID() shared.OptionID { return o.id }

// Name returns the option's name, trimmed.
func (o CustomizationOption) Name() string { return o.name }

// Required reports whether the client must pick a choice.
func (o CustomizationOption) Required() bool { return o.required }

// Choices returns the choices in order. The result is a copy.
func (o CustomizationOption) Choices() []Choice { return cloneSlice(o.choices) }

// NewOption returns the option described by in. Names and labels are
// trimmed. It returns a ValidationError listing every invalid field:
// "name" (1 to 80 characters), "choices" (1 to 50 of them), and, for the
// choice at index i, "choices[i].label" (1 to 80 characters, unique within
// the option ignoring case), "choices[i].extraPriceCents" (zero or more, and
// not above R$ 1.000.000.000,00) and "choices[i].extraDays" (0 to 365). It
// returns ErrInvalidOption for a zero option or choice id, or an extra price
// with no currency, which only a bug can cause.
func NewOption(in OptionInput) (CustomizationOption, error) {
	if in.ID.IsZero() {
		return CustomizationOption{}, fmt.Errorf("new option: zero id: %w", ErrInvalidOption)
	}
	name := strings.TrimSpace(in.Name)
	choices, cerr := buildChoices(in.Choices)
	if err := shared.MergeValidation(optionRule.check(name), cerr); err != nil {
		return CustomizationOption{}, err
	}
	return CustomizationOption{id: in.ID, name: name, required: in.Required, choices: choices}, nil
}

func buildChoices(in []ChoiceInput) ([]Choice, error) {
	switch {
	case len(in) == 0:
		return nil, fieldError(fieldChoices, msgNoChoices)
	case len(in) > maxChoices:
		return nil, fieldError(fieldChoices, msgTooManyChoices)
	}
	choices := make([]Choice, 0, len(in))
	labels := make(map[string]struct{}, len(in))
	var errs []error
	for i, ci := range in {
		c, err := buildChoice(ci)
		if err != nil {
			errs = append(errs, prefixFields(err, at(fieldChoices, i)))
			continue
		}
		key := strings.ToLower(c.label)
		if _, dup := labels[key]; dup {
			errs = append(errs, prefixFields(fieldError(fieldLabel, msgChoiceRepeated), at(fieldChoices, i)))
			continue
		}
		labels[key] = struct{}{}
		choices = append(choices, c)
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return nil, err
	}
	return choices, nil
}

func buildChoice(in ChoiceInput) (Choice, error) {
	if in.ID.IsZero() || in.ExtraPrice.Currency() == "" {
		return Choice{}, fmt.Errorf("choice %v: zero id or no currency: %w", in.ID, ErrInvalidOption)
	}
	label := strings.TrimSpace(in.Label)
	err := shared.MergeValidation(choiceRule.check(label), checkExtra(in))
	if err != nil {
		return Choice{}, err
	}
	return Choice{id: in.ID, label: label, extraPrice: in.ExtraPrice, extraDays: in.ExtraDays}, nil
}

func checkExtra(in ChoiceInput) error {
	var errs []error
	if in.ExtraPrice.IsNegative() {
		errs = append(errs, fieldError(fieldExtraPrice, msgExtraPrice))
	} else {
		errs = append(errs, checkAmount(fieldExtraPrice, in.ExtraPrice))
	}
	if in.ExtraDays < 0 || in.ExtraDays > maxDays {
		errs = append(errs, fieldError(fieldExtraDays, msgExtraDays))
	}
	return shared.MergeValidation(errs...)
}

// buildOptions builds the options of an item priced in currency. It returns
// the errors of NewOption with each field prefixed by "options[i].", a
// ValidationError on "options" for more than 20, and ErrInvalidOption for an
// option id used twice or an extra price in another currency.
func buildOptions(in []OptionInput, currency string) ([]CustomizationOption, error) {
	if len(in) > maxOptions {
		return nil, fieldError(fieldOptions, msgTooManyOptions)
	}
	options := make([]CustomizationOption, 0, len(in))
	ids := make([]shared.OptionID, 0, len(in))
	var errs []error
	for i, oi := range in {
		o, err := NewOption(oi)
		if err != nil {
			errs = append(errs, prefixFields(err, at(fieldOptions, i)))
			continue
		}
		if !optionInCurrency(o, currency) {
			return nil, fmt.Errorf("option %v: extra price not in %s: %w", o.id, currency, ErrInvalidOption)
		}
		options = append(options, o)
		ids = append(ids, o.id)
	}
	if hasDuplicate(ids) {
		return nil, fmt.Errorf("an option id is used twice: %w", ErrInvalidOption)
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return nil, err
	}
	return options, nil
}

func optionInCurrency(o CustomizationOption, currency string) bool {
	for _, c := range o.choices {
		if !sameCurrency(c.extraPrice, currency) {
			return false
		}
	}
	return true
}

// PriceWithOptions returns what a loose item costs with the client's
// choices: (unit price + every chosen extra) × quantity (ADR 0024). It
// returns shared.ErrCurrencyMismatch when an extra is in another currency
// and shared.ErrMoneyOverflow when the result does not fit.
func PriceWithOptions(unit shared.Money, extras []shared.Money, quantity int) (shared.Money, error) {
	each, err := AddOptionExtras(unit, extras, 1)
	if err != nil {
		return shared.Money{}, err
	}
	return times(each, quantity)
}

// AddOptionExtras returns base + every extra × quantity: a kit's price with
// the choices made for one of its items, which the kit holds quantity of.
// Errors are those of PriceWithOptions.
func AddOptionExtras(base shared.Money, extras []shared.Money, quantity int) (shared.Money, error) {
	for _, e := range extras {
		part, err := times(e, quantity)
		if err != nil {
			return shared.Money{}, err
		}
		if base, err = base.Add(part); err != nil {
			return shared.Money{}, err
		}
	}
	return base, nil
}
