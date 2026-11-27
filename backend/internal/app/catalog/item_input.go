package catalog

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// itemDomainInput turns the client's item into the domain's input, in the
// store's currency. current are the item's options (none for a new item):
// an option or choice keeps its id only when the item has it.
func (s *Service) itemDomainInput(in ItemInput, currency string, current []catalog.CustomizationOption) (catalog.ItemInput, error) {
	price, err := shared.NewMoney(in.PriceCents, currency)
	if err != nil {
		return catalog.ItemInput{}, err
	}
	options, err := s.optionInputs(in.Options, current, currency)
	if err != nil {
		return catalog.ItemInput{}, err
	}
	sourcing, err := sourcingInput(in, currency)
	if err != nil {
		return catalog.ItemInput{}, err
	}
	return catalog.ItemInput{
		ItemDetails: catalog.ItemDetails{
			Name: in.Name, Description: in.Description, Category: in.CategoryID, Price: price,
			AttributeValues: in.AttributeValueIDs, Options: options,
		},
		Sourcing: sourcing,
	}, nil
}

func sourcingInput(in ItemInput, currency string) (catalog.SourcingInput, error) {
	production, err := in.ProductionCost.change(currency)
	if err != nil {
		return catalog.SourcingInput{}, err
	}
	main, err := linkInput(in.MainSupplier, currency)
	if err != nil {
		return catalog.SourcingInput{}, err
	}
	backup, err := linkInput(in.BackupSupplier, currency)
	if err != nil {
		return catalog.SourcingInput{}, err
	}
	return catalog.SourcingInput{
		Mode: in.SourcingMode, ProductionDays: in.ProductionDays, ProductionCost: production, Main: main, Backup: backup,
	}, nil
}

func linkInput(in *SupplierInput, currency string) (*catalog.SupplierLink, error) {
	if in == nil {
		return nil, nil
	}
	cost, err := in.Cost.change(currency)
	if err != nil {
		return nil, err
	}
	return &catalog.SupplierLink{Supplier: in.SupplierID, LeadDays: in.LeadDays, Cost: cost}, nil
}

// optionInputs turns the client's options into the domain's. An option keeps
// its id only when it is one of current, the first time it is sent; so do
// its choices within the kept option. Every other option or choice gets a
// new id, so a client can never reuse another item's option.
func (s *Service) optionInputs(in []OptionInput, current []catalog.CustomizationOption, currency string) ([]catalog.OptionInput, error) {
	known := make(map[shared.OptionID]map[shared.ChoiceID]bool, len(current))
	for _, o := range current {
		choices := map[shared.ChoiceID]bool{}
		for _, c := range o.Choices() {
			choices[c.ID()] = true
		}
		known[o.ID()] = choices
	}
	out := make([]catalog.OptionInput, 0, len(in))
	for _, o := range in {
		id, choices := o.ID, known[o.ID]
		if choices == nil {
			id = shared.OptionID(s.ids.NewID())
		}
		delete(known, o.ID) // a second use of the same id is a new option
		opt := catalog.OptionInput{ID: id, Name: o.Name, Required: o.Required, Choices: make([]catalog.ChoiceInput, 0, len(o.Choices))}
		for _, c := range o.Choices {
			extra, err := shared.NewMoney(c.ExtraPriceCents, currency)
			if err != nil {
				return nil, err
			}
			cid := c.ID
			if !choices[cid] {
				cid = shared.ChoiceID(s.ids.NewID())
			}
			delete(choices, c.ID)
			opt.Choices = append(opt.Choices, catalog.ChoiceInput{ID: cid, Label: c.Label, ExtraPrice: extra, ExtraDays: c.ExtraDays})
		}
		out = append(out, opt)
	}
	return out, nil
}
