package catalog

import (
	"errors"
	"math"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// validOption returns an option named "Cor da costura" with two choices.
func validOption(t testing.TB, n byte) OptionInput {
	return OptionInput{
		ID:       optionID(n),
		Name:     "Cor da costura",
		Required: true,
		Choices: []ChoiceInput{
			{ID: choiceID(1), Label: "Vermelha", ExtraPrice: moneyOf(t, 0), ExtraDays: 0},
			{ID: choiceID(2), Label: "Azul", ExtraPrice: moneyOf(t, 2500), ExtraDays: 3},
		},
	}
}

func choices(t testing.TB, n int) []ChoiceInput {
	out := make([]ChoiceInput, n)
	for i := range out {
		out[i] = ChoiceInput{
			ID:         shared.ChoiceID(shared.NewV7(int64(i+1), [10]byte{byte(i), 1})),
			Label:      "Escolha " + string(rune('A'+i/26)) + string(rune('a'+i%26)),
			ExtraPrice: moneyOf(t, 0),
		}
	}
	return out
}

func TestNewOption(t *testing.T) {
	t.Parallel()

	in := validOption(t, 1)
	in.Name = "  Cor da costura  "
	in.Choices[0].Label = "  Vermelha  "
	o, err := NewOption(in)
	if err != nil {
		t.Fatal(err)
	}
	if o.ID() != optionID(1) || o.Name() != "Cor da costura" || !o.Required() {
		t.Errorf("option = %v %q required %v", o.ID(), o.Name(), o.Required())
	}
	got := o.Choices()
	if len(got) != 2 || got[0].Label() != "Vermelha" || got[0].ID() != choiceID(1) {
		t.Fatalf("choices = %+v", got)
	}
	if got[1].ExtraPrice().Cents() != 2500 || got[1].ExtraDays() != 3 {
		t.Errorf("second choice adds %v and %d days, want 2500 and 3", got[1].ExtraPrice(), got[1].ExtraDays())
	}
	got[0] = Choice{}
	if o.Choices()[0].Label() != "Vermelha" {
		t.Error("changing the returned choices changed the option")
	}
}

func TestNewOption_Optional(t *testing.T) {
	t.Parallel()

	in := validOption(t, 1)
	in.Required = false
	o, err := NewOption(in)
	if err != nil || o.Required() {
		t.Errorf("optional option = required %v, error %v", o.Required(), err)
	}
}

func TestNewOption_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(in *OptionInput)
		want   map[string]string
	}{
		{"blank name", func(in *OptionInput) { in.Name = " " }, map[string]string{"name": msgOptionNameRequired}},
		{"name over 80", func(in *OptionInput) { in.Name = repeat("a", 81) }, map[string]string{"name": msgShortNameTooLong}},
		{"name with a control character", func(in *OptionInput) { in.Name = "a\tb" }, map[string]string{"name": msgNameControl}},
		{"no choices", func(in *OptionInput) { in.Choices = nil }, map[string]string{"choices": msgNoChoices}},
		{"51 choices", func(in *OptionInput) { in.Choices = choices(t, 51) }, map[string]string{"choices": msgTooManyChoices}},
		{"blank label", func(in *OptionInput) { in.Choices[1].Label = "  " }, map[string]string{"choices[1].label": msgChoiceRequired}},
		{"label over 80", func(in *OptionInput) { in.Choices[0].Label = repeat("a", 81) }, map[string]string{"choices[0].label": msgChoiceTooLong}},
		{"label with a line break", func(in *OptionInput) { in.Choices[0].Label = "a\nb" }, map[string]string{"choices[0].label": msgChoiceControl}},
		{"repeated label", func(in *OptionInput) { in.Choices[1].Label = "vermelha" }, map[string]string{"choices[1].label": msgChoiceRepeated}},
		{"repeated label after trimming", func(in *OptionInput) { in.Choices[1].Label = " VERMELHA " }, map[string]string{"choices[1].label": msgChoiceRepeated}},
		{"negative extra price", func(in *OptionInput) { in.Choices[0].ExtraPrice = moneyOf(t, -1) }, map[string]string{"choices[0].extraPriceCents": msgExtraPrice}},
		{"extra price too high", func(in *OptionInput) { in.Choices[0].ExtraPrice = moneyOf(t, maxAmountCents+1) }, map[string]string{"choices[0].extraPriceCents": msgAmountTooHigh}},
		{"negative extra days", func(in *OptionInput) { in.Choices[1].ExtraDays = -1 }, map[string]string{"choices[1].extraDays": msgExtraDays}},
		{"extra days over 365", func(in *OptionInput) { in.Choices[1].ExtraDays = 366 }, map[string]string{"choices[1].extraDays": msgExtraDays}},
		{"everything wrong", func(in *OptionInput) {
			in.Name = ""
			in.Choices[0].ExtraDays = 999
			in.Choices[1].Label = ""
		}, map[string]string{"name": msgOptionNameRequired, "choices[0].extraDays": msgExtraDays, "choices[1].label": msgChoiceRequired}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := validOption(t, 1)
			tt.mutate(&in)
			_, err := NewOption(in)
			wantFields(t, err, tt.want)
		})
	}
}

func TestNewOption_Boundaries(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(in *OptionInput){
		"name of 80 characters":  func(in *OptionInput) { in.Name = repeat("é", 80) },
		"label of 80 characters": func(in *OptionInput) { in.Choices[0].Label = repeat("é", 80) },
		"one choice":             func(in *OptionInput) { in.Choices = in.Choices[:1] },
		"fifty choices":          func(in *OptionInput) { in.Choices = choices(t, 50) },
		"365 extra days":         func(in *OptionInput) { in.Choices[0].ExtraDays = 365 },
		"highest extra price":    func(in *OptionInput) { in.Choices[0].ExtraPrice = moneyOf(t, maxAmountCents) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := validOption(t, 1)
			mutate(&in)
			if _, err := NewOption(in); err != nil {
				t.Errorf("NewOption error = %v, want none", err)
			}
		})
	}
}

func TestNewOption_ProgrammingErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]func(in *OptionInput){
		"zero option id":               func(in *OptionInput) { in.ID = shared.OptionID{} },
		"zero choice id":               func(in *OptionInput) { in.Choices[0].ID = shared.ChoiceID{} },
		"extra price with no currency": func(in *OptionInput) { in.Choices[0].ExtraPrice = shared.Money{} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := validOption(t, 1)
			mutate(&in)
			_, err := NewOption(in)
			if !errors.Is(err, ErrInvalidOption) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalidOption and not a 400", err)
			}
		})
	}
}

func FuzzNewOption(f *testing.F) {
	f.Add("Cor", "Vermelha", "vermelha", int64(0), 0)
	f.Add("", "", "", int64(-1), -1)
	f.Add("a\x00b", "é", "É", int64(maxAmountCents+1), 366)
	f.Add("  x  ", "\xff", " ", int64(100), 365)
	f.Fuzz(func(t *testing.T, name, label1, label2 string, cents int64, days int) {
		price, err := shared.NewMoney(cents, "BRL")
		if err != nil {
			t.Fatal(err)
		}
		o, err := NewOption(OptionInput{
			ID: optionID(1), Name: name,
			Choices: []ChoiceInput{
				{ID: choiceID(1), Label: label1, ExtraPrice: price, ExtraDays: days},
				{ID: choiceID(2), Label: label2, ExtraPrice: price, ExtraDays: days},
			},
		})
		if err != nil {
			var ve *shared.ValidationError
			if !errors.As(err, &ve) || len(ve.Fields) == 0 {
				t.Fatalf("error = %v, want a ValidationError with fields", err)
			}
			return
		}
		checkOptionInvariants(t, o)
	})
}

// checkOptionInvariants fails the test when a built option breaks a rule of
// ADR 0024.
func checkOptionInvariants(t *testing.T, o CustomizationOption) {
	t.Helper()
	if n := utf8.RuneCountInString(o.Name()); n < 1 || n > 80 || o.Name() != strings.TrimSpace(o.Name()) {
		t.Errorf("name %q is not trimmed, 1 to 80 characters", o.Name())
	}
	seen := map[string]bool{}
	got := o.Choices()
	if len(got) < 1 || len(got) > 50 {
		t.Errorf("%d choices, want 1 to 50", len(got))
	}
	for _, c := range got {
		label := c.Label()
		if n := utf8.RuneCountInString(label); n < 1 || n > 80 || label != strings.TrimSpace(label) || !utf8.ValidString(label) {
			t.Errorf("label %q is not a trimmed, valid, 1 to 80 character text", label)
		}
		if strings.ContainsFunc(label, unicode.IsControl) {
			t.Errorf("label %q has a control character", label)
		}
		if seen[strings.ToLower(label)] {
			t.Errorf("label %q is repeated", label)
		}
		seen[strings.ToLower(label)] = true
		if c.ExtraPrice().IsNegative() || c.ExtraPrice().Cents() > maxAmountCents || c.ExtraDays() < 0 || c.ExtraDays() > 365 {
			t.Errorf("choice %q adds %v and %d days", label, c.ExtraPrice(), c.ExtraDays())
		}
	}
}

func TestPriceWithOptions_UnitPlusExtrasTimesQuantity(t *testing.T) {
	t.Parallel()
	brl := func(c int64) shared.Money { m, _ := shared.NewMoney(c, "BRL"); return m }
	got, err := PriceWithOptions(brl(10_000), []shared.Money{brl(500), brl(250)}, 3)
	if err != nil || got.Cents() != 32_250 {
		t.Errorf("PriceWithOptions = %v %v, want (100,00 + 5,00 + 2,50) × 3", got, err)
	}
	kit, err := AddOptionExtras(brl(29_000), []shared.Money{brl(500)}, 2)
	if err != nil || kit.Cents() != 30_000 {
		t.Errorf("AddOptionExtras = %v %v, want 290,00 + 5,00 × 2", kit, err)
	}
	usd, _ := shared.NewMoney(1, "USD")
	if _, err := PriceWithOptions(brl(1), []shared.Money{usd}, 1); !errors.Is(err, shared.ErrCurrencyMismatch) {
		t.Errorf("mixed currency: err = %v", err)
	}
	if _, err := PriceWithOptions(brl(math.MaxInt64/2), nil, 3); !errors.Is(err, shared.ErrMoneyOverflow) {
		t.Errorf("overflow: err = %v", err)
	}
}
