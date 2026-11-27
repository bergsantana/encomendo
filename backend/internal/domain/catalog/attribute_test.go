package catalog

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func servesIn() AttributeInput {
	return AttributeInput{Name: "  Serve em  ", Values: []ValueInput{
		{ID: attrValueID(1), Label: " CB 500X "},
		{ID: attrValueID(2), Label: "XRE 300"},
	}}
}

func TestNewAttribute(t *testing.T) {
	t.Parallel()

	a, err := NewAttribute(tenantID, attributeID(1), servesIn())
	if err != nil {
		t.Fatal(err)
	}
	if a.TenantID() != tenantID || a.ID() != attributeID(1) || a.Name() != "Serve em" || a.Version() != 1 {
		t.Errorf("attribute = %v %v %q v%d", a.TenantID(), a.ID(), a.Name(), a.Version())
	}
	values := a.Values()
	if len(values) != 2 || values[0].Label() != "CB 500X" || values[0].ID() != attrValueID(1) || values[1].Label() != "XRE 300" {
		t.Errorf("values = %+v, want trimmed and in order", values)
	}
	values[0] = AttributeValue{}
	if a.Values()[0].Label() != "CB 500X" {
		t.Error("changing the returned values changed the attribute")
	}
	if _, err := NewAttribute(tenantID, attributeID(2), AttributeInput{Name: "Cor"}); err != nil {
		t.Errorf("a field with no values yet = %v, want none", err)
	}
}

func TestNewAttribute_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(in *AttributeInput)
		want   map[string]string
	}{
		{"blank name", func(in *AttributeInput) { in.Name = " " }, map[string]string{"name": msgAttributeNameMissing}},
		{"name over 120", func(in *AttributeInput) { in.Name = repeat("a", 121) }, map[string]string{"name": msgNameTooLong}},
		{"blank value", func(in *AttributeInput) { in.Values[1].Label = "" }, map[string]string{"values[1].label": msgValueRequired}},
		{"value over 80", func(in *AttributeInput) { in.Values[0].Label = repeat("a", 81) }, map[string]string{"values[0].label": msgValueTooLong}},
		{"value with a control character", func(in *AttributeInput) { in.Values[0].Label = "a\x00" }, map[string]string{"values[0].label": msgValueControl}},
		{"repeated value", func(in *AttributeInput) { in.Values[1].Label = "cb 500x" }, map[string]string{"values[1].label": msgValueDuplicate}},
		{"more than 200 values", func(in *AttributeInput) {
			in.Values = nil
			for n := range 201 {
				in.Values = append(in.Values, ValueInput{ID: shared.AttributeValueID(shared.NewV7(int64(n+1), [10]byte{byte(n), 9})), Label: repeat("x", n+1)})
			}
		}, map[string]string{"values": msgTooManyAttrVals}},
		{"name and value", func(in *AttributeInput) { in.Name = ""; in.Values[0].Label = "" }, map[string]string{"name": msgAttributeNameMissing, "values[0].label": msgValueRequired}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := servesIn()
			tt.mutate(&in)
			_, err := NewAttribute(tenantID, attributeID(1), in)
			wantFields(t, err, tt.want)
		})
	}
}

func TestNewAttribute_ProgrammingErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]func(tn *shared.TenantID, id *shared.AttributeID, in *AttributeInput){
		"zero tenant": func(tn *shared.TenantID, _ *shared.AttributeID, _ *AttributeInput) { *tn = shared.TenantID{} },
		"zero id":     func(_ *shared.TenantID, id *shared.AttributeID, _ *AttributeInput) { *id = shared.AttributeID{} },
		"zero value id": func(_ *shared.TenantID, _ *shared.AttributeID, in *AttributeInput) {
			in.Values[0].ID = shared.AttributeValueID{}
		},
		"repeated value id": func(_ *shared.TenantID, _ *shared.AttributeID, in *AttributeInput) { in.Values[1].ID = in.Values[0].ID },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tn, id, in := tenantID, attributeID(1), servesIn()
			mutate(&tn, &id, &in)
			_, err := NewAttribute(tn, id, in)
			if !errors.Is(err, ErrInvalidAttribute) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalidAttribute and not a 400", err)
			}
		})
	}
}

func TestAttribute_Replace(t *testing.T) {
	t.Parallel()

	a, err := NewAttribute(tenantID, attributeID(1), servesIn())
	if err != nil {
		t.Fatal(err)
	}
	removed, err := a.Replace(AttributeInput{Name: "Compatível com", Values: []ValueInput{
		{ID: attrValueID(2), Label: "XRE 300 Sahara"}, // kept, renamed
		{ID: attrValueID(3), Label: "Bros"},           // new
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != attrValueID(1) {
		t.Errorf("removed = %v, want just the value left out", removed)
	}
	if a.Name() != "Compatível com" || len(a.Values()) != 2 || a.Values()[0].ID() != attrValueID(2) || a.Values()[0].Label() != "XRE 300 Sahara" {
		t.Errorf("after Replace: %q %+v", a.Name(), a.Values())
	}

	removed, err = a.Replace(AttributeInput{Name: "x"})
	if err != nil || len(removed) != 2 {
		t.Errorf("removing every value: removed %v, error %v", removed, err)
	}

	_, err = a.Replace(AttributeInput{Name: ""})
	wantFields(t, err, map[string]string{"name": msgAttributeNameMissing})
	if a.Name() != "x" {
		t.Error("a refused Replace changed the field")
	}
}

func TestAttribute_CheckVersion(t *testing.T) {
	t.Parallel()

	a, err := NewAttribute(tenantID, attributeID(1), servesIn())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CheckVersion(1); err != nil {
		t.Errorf("CheckVersion(1) = %v", err)
	}
	if err := a.CheckVersion(3); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("CheckVersion(3) = %v, want ErrVersionConflict", err)
	}
}

func TestRestoreAttribute(t *testing.T) {
	t.Parallel()

	rec := AttributeRecord{Tenant: tenantID, ID: attributeID(1), Name: "Serve em", Version: 4,
		Values: []ValueInput{{ID: attrValueID(1), Label: "CB 500X"}}}
	a, err := RestoreAttribute(rec)
	if err != nil || a.Version() != 4 || len(a.Values()) != 1 {
		t.Fatalf("RestoreAttribute = %+v, %v", a, err)
	}
	for name, mutate := range map[string]func(r *AttributeRecord){
		"version 0":       func(r *AttributeRecord) { r.Version = 0 },
		"untrimmed name":  func(r *AttributeRecord) { r.Name = "Serve em " },
		"untrimmed value": func(r *AttributeRecord) { r.Values[0].Label = " CB" },
		"blank name":      func(r *AttributeRecord) { r.Name = "" },
		"repeated labels": func(r *AttributeRecord) {
			r.Values = append(r.Values, ValueInput{ID: attrValueID(2), Label: "cb 500x"})
		},
	} {
		r := rec
		r.Values = append([]ValueInput(nil), rec.Values...)
		mutate(&r)
		_, err := RestoreAttribute(r)
		if !errors.Is(err, ErrInvalidAttribute) || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalidAttribute and not a 400", name, err)
		}
	}
}
