package catalog

import (
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ValueInput is one value of an item field as a client of the domain sends
// it. The caller keeps the id of a value that already exists and generates
// one for a new value.
type ValueInput struct {
	ID    shared.AttributeValueID
	Label string
}

// AttributeInput is an item field as a client of the domain sends it.
type AttributeInput struct {
	Name   string
	Values []ValueInput
}

// AttributeValue is one value of an item field, such as "CB 500X". It is an
// immutable value.
type AttributeValue struct {
	id    shared.AttributeValueID
	label string
}

// ID returns the value's id.
func (v AttributeValue) ID() shared.AttributeValueID { return v.id }

// Label returns the value's text, trimmed.
func (v AttributeValue) Label() string { return v.label }

// Attribute is an item field such as "Serve em": a name and an ordered list
// of values that items and kits pick (ADR 0025), so the store page can
// filter by them. The store creates its fields itself (ADR 0024).
type Attribute struct {
	tenant  shared.TenantID
	id      shared.AttributeID
	name    string
	values  []AttributeValue
	version int
}

// NewAttribute returns an item field at version 1. Names and labels are
// trimmed. It returns a ValidationError listing every invalid field: "name"
// (1 to 120 characters), "values" (at most 200) and, for the value at index
// i, "values[i].label" (1 to 80 characters, unique within the field ignoring
// case). It returns ErrInvalidAttribute for a zero tenant, field or value id.
func NewAttribute(tenant shared.TenantID, id shared.AttributeID, in AttributeInput) (*Attribute, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new attribute %v: %w", id, ErrInvalidAttribute)
	}
	name, values, err := buildAttribute(in)
	if err != nil {
		return nil, err
	}
	return &Attribute{tenant: tenant, id: id, name: name, values: values, version: 1}, nil
}

func buildAttribute(in AttributeInput) (string, []AttributeValue, error) {
	name := strings.TrimSpace(in.Name)
	values, verr := buildValues(in.Values)
	if err := shared.MergeValidation(attributeRule.check(name), verr); err != nil {
		return "", nil, err
	}
	return name, values, nil
}

func buildValues(in []ValueInput) ([]AttributeValue, error) {
	if len(in) > maxAttributeValues {
		return nil, fieldError(fieldValues, msgTooManyAttrVals)
	}
	values := make([]AttributeValue, 0, len(in))
	labels := make(map[string]struct{}, len(in))
	ids := make(map[shared.AttributeValueID]struct{}, len(in))
	var errs []error
	for i, vi := range in {
		if vi.ID.IsZero() {
			return nil, fmt.Errorf("attribute value %d has no id: %w", i, ErrInvalidAttribute)
		}
		if _, dup := ids[vi.ID]; dup {
			return nil, fmt.Errorf("attribute value id %v used twice: %w", vi.ID, ErrInvalidAttribute)
		}
		ids[vi.ID] = struct{}{}
		label := strings.TrimSpace(vi.Label)
		err := valueRule.check(label)
		if _, dup := labels[strings.ToLower(label)]; err == nil && dup {
			err = fieldError(fieldLabel, msgValueDuplicate)
		}
		labels[strings.ToLower(label)] = struct{}{}
		if err != nil {
			errs = append(errs, prefixFields(err, at(fieldValues, i)))
			continue
		}
		values = append(values, AttributeValue{id: vi.ID, label: label})
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return nil, err
	}
	return values, nil
}

// AttributeRecord is an item field as a repository stores it.
type AttributeRecord struct {
	Tenant  shared.TenantID
	ID      shared.AttributeID
	Name    string
	Values  []ValueInput
	Version int
}

// RestoreAttribute rebuilds a stored item field. It returns
// ErrInvalidAttribute when the record breaks a rule a stored field always
// keeps.
func RestoreAttribute(r AttributeRecord) (*Attribute, error) {
	bad := r.Version < 1 || r.Name != strings.TrimSpace(r.Name)
	for _, v := range r.Values {
		bad = bad || v.Label != strings.TrimSpace(v.Label)
	}
	if bad {
		return nil, fmt.Errorf("restore attribute %v: %w", r.ID, ErrInvalidAttribute)
	}
	a, err := NewAttribute(r.Tenant, r.ID, AttributeInput{Name: r.Name, Values: r.Values})
	if err != nil {
		// The cause is text only: wrapping a ValidationError would make a bad
		// stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore attribute %v: %w: %s", r.ID, ErrInvalidAttribute, err.Error())
	}
	a.version = r.Version
	return a, nil
}

// TenantID returns the store the field belongs to.
func (a *Attribute) TenantID() shared.TenantID { return a.tenant }

// ID returns the field's id.
func (a *Attribute) ID() shared.AttributeID { return a.id }

// Name returns the field's name, such as "Serve em", trimmed.
func (a *Attribute) Name() string { return a.name }

// Values returns the values in the store's order. The result is a copy.
func (a *Attribute) Values() []AttributeValue { return cloneSlice(a.values) }

// Version returns the version the field was read or created at.
func (a *Attribute) Version() int { return a.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// version the field was read at.
func (a *Attribute) CheckVersion(expected int) error {
	if expected != a.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// Replace gives the field a new name and values, as the contract's
// AttributeInput does: a value sent with a known id keeps it, a new id adds
// a value, and a value left out is removed. It returns the ids of the removed
// values, so the app layer can take them off the items and kits that picked
// them (after the confirmation the product asks for), and the errors of
// NewAttribute; nothing changes then.
func (a *Attribute) Replace(in AttributeInput) (removed []shared.AttributeValueID, err error) {
	name, values, err := buildAttribute(in)
	if err != nil {
		return nil, err
	}
	kept := make(map[shared.AttributeValueID]struct{}, len(values))
	for _, v := range values {
		kept[v.id] = struct{}{}
	}
	for _, old := range a.values {
		if _, ok := kept[old.id]; !ok {
			removed = append(removed, old.id)
		}
	}
	a.name, a.values = name, values
	return removed, nil
}
