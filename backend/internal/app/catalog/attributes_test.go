package catalog_test

import (
	"errors"
	"slices"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func (h *harness) attribute(t *testing.T, name string, labels ...string) appcatalog.AttributeView {
	t.Helper()
	in := appcatalog.AttributeInput{Name: name}
	for _, l := range labels {
		in.Values = append(in.Values, appcatalog.ValueInput{Label: l})
	}
	a, err := h.svc.CreateAttribute(h.admin(t), in)
	if err != nil {
		t.Fatalf("CreateAttribute(%s): %v", name, err)
	}
	return a
}

func labels(a appcatalog.AttributeView) []string {
	var out []string
	for _, v := range a.Values {
		out = append(out, v.Label)
	}
	return out
}

func TestCreateAttribute_AddsAtTheEndWithNewValueIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	first := h.attribute(t, "Serve em", "CB 500X", "XRE 300")
	stray := shared.AttributeValueID(h.ids.NewID())
	second, err := h.svc.CreateAttribute(h.admin(t), appcatalog.AttributeInput{
		Name: "Material", Values: []appcatalog.ValueInput{{ID: first.Values[0].ID, Label: "Couro"}, {ID: stray, Label: "Tecido"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.Position != 0 || second.Position != 1 || !slices.Equal(labels(first), []string{"CB 500X", "XRE 300"}) {
		t.Errorf("created = %+v, %+v", first, second)
	}
	for _, v := range second.Values {
		if v.ID == first.Values[0].ID || v.ID == stray || v.ID.IsZero() {
			t.Errorf("value %s kept a client id %s; a new field's values get new ids", v.Label, v.ID)
		}
	}
	if _, err := h.svc.CreateAttribute(h.admin(t), appcatalog.AttributeInput{Name: "serve em"}); !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("a duplicate name: %v, want ErrNameTaken", err)
	}
	_, err = h.svc.CreateAttribute(h.admin(t), appcatalog.AttributeInput{Name: "Cor", Values: []appcatalog.ValueInput{{Label: "Azul"}, {Label: "azul"}}})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"values[1].label"}) {
		t.Errorf("a repeated label: fields %v", f)
	}
}

func TestReplaceAttribute_KeepsValuesAndDropsTheRemoved(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.attribute(t, "Serve em", "CB 500X", "XRE 300")
	cb, xre := a.Values[0].ID, a.Values[1].ID
	in := inHouseInput("Banco", 10000, 5)
	in.AttributeValueIDs = []shared.AttributeValueID{cb, xre}
	it := h.createItem(t, in)
	k := kitOf("Kit", 9000, one(it.ID))
	k.AttributeValueIDs = []shared.AttributeValueID{xre}
	kit := h.createKit(t, k)

	listed, err := h.svc.ListAttributes(h.viewer(t))
	if err != nil || len(listed) != 1 || listed[0].Values[0].UsageCount != 1 || listed[0].Values[1].UsageCount != 2 {
		t.Fatalf("ListAttributes = %+v, %v; want CB 500X used once, XRE 300 twice", listed, err)
	}
	got, err := h.svc.ReplaceAttribute(h.admin(t), appcatalog.ReplaceAttributeInput{ID: a.ID, Attribute: appcatalog.AttributeInput{
		Name: "Moto", Values: []appcatalog.ValueInput{{Label: "NC 750X"}, {ID: cb, Label: "CB 500 X"}, {ID: cb, Label: "CB 500F"}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if got.Name != "Moto" || !slices.Equal(labels(got), []string{"NC 750X", "CB 500 X", "CB 500F"}) || got.Values[1].ID != cb {
		t.Errorf("replaced = %+v; want CB 500X kept by id and renamed", got)
	}
	if got.Values[2].ID == cb || got.Values[1].UsageCount != 1 || got.Values[0].UsageCount != 0 {
		t.Errorf("replaced values = %+v; a repeated id is a new value, and usage follows the ids", got.Values)
	}
	item, _ := h.svc.GetItem(h.viewer(t), it.ID)
	gotKit, _ := h.svc.GetKit(h.viewer(t), kit.ID)
	if !slices.Equal(item.AttributeValueIDs, []shared.AttributeValueID{cb}) || item.Version != 1 {
		t.Errorf("the item after XRE 300 left = %v at version %d", item.AttributeValueIDs, item.Version)
	}
	if len(gotKit.AttributeValueIDs) != 0 || gotKit.Version != 1 {
		t.Errorf("the kit after XRE 300 left = %v at version %d", gotKit.AttributeValueIDs, gotKit.Version)
	}
}

func TestReplaceAttribute_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.attribute(t, "Serve em")
	h.attribute(t, "Material")
	ctx := h.admin(t)

	if _, err := h.svc.ReplaceAttribute(ctx, appcatalog.ReplaceAttributeInput{ID: a.ID, Attribute: appcatalog.AttributeInput{Name: "MATERIAL"}}); !errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("onto another name: %v, want ErrNameTaken", err)
	}
	_, err := h.svc.ReplaceAttribute(ctx, appcatalog.ReplaceAttributeInput{ID: a.ID, Attribute: appcatalog.AttributeInput{Name: ""}})
	if f := fieldsOf(t, err); !slices.Equal(f, []string{"name"}) {
		t.Errorf("a blank name: fields %v", f)
	}
	if _, err := h.svc.ReplaceAttribute(ctx, appcatalog.ReplaceAttributeInput{ID: shared.AttributeID(h.ids.NewID()), Attribute: appcatalog.AttributeInput{Name: "X"}}); !errors.Is(err, catalog.ErrAttributeNotFound) {
		t.Errorf("an unknown field: %v, want ErrAttributeNotFound", err)
	}
	h.store.Fail("AttributesUsageCounts", errBoom)
	if _, err := h.svc.ReplaceAttribute(ctx, appcatalog.ReplaceAttributeInput{ID: a.ID, Attribute: appcatalog.AttributeInput{Name: "Y"}}); !errors.Is(err, errBoom) {
		t.Errorf("a failed count: %v", err)
	}
	if _, err := h.svc.ListAttributes(ctx); !errors.Is(err, errBoom) {
		t.Errorf("ListAttributes with a failed count: %v", err)
	}
	h.store.Fail("AttributesList", errBoom)
	if _, err := h.svc.ListAttributes(ctx); !errors.Is(err, errBoom) {
		t.Errorf("ListAttributes: %v", err)
	}
	if _, err := h.svc.CreateAttribute(ctx, appcatalog.AttributeInput{Name: "Z"}); !errors.Is(err, errBoom) {
		t.Errorf("CreateAttribute: %v", err)
	}
	h.store.Fail("AttributesList", nil)
	h.store.Fail("AttributesUsageCounts", nil)
	if got, _ := h.svc.ListAttributes(ctx); got[0].Name != "Serve em" {
		t.Errorf("after the refusals: %+v, want nothing changed", got)
	}
}

func TestDeleteAttribute_ValuesLeaveItems(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.attribute(t, "Serve em", "CB 500X")
	in := inHouseInput("Banco", 10000, 5)
	in.AttributeValueIDs = []shared.AttributeValueID{a.Values[0].ID}
	it := h.createItem(t, in)

	if err := h.svc.DeleteAttribute(h.admin(t), a.ID); err != nil {
		t.Fatal(err)
	}

	if item, _ := h.svc.GetItem(h.viewer(t), it.ID); len(item.AttributeValueIDs) != 0 {
		t.Errorf("the item still has %v", item.AttributeValueIDs)
	}
	if err := h.svc.DeleteAttribute(h.admin(t), a.ID); !errors.Is(err, catalog.ErrAttributeNotFound) {
		t.Errorf("deleting it again: %v, want ErrAttributeNotFound", err)
	}
}
