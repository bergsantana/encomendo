package handler_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// newAddressBody is a create body for delivery to a "Novo endereço"; edit
// changes the address object before it is sent.
func (r *visRig) newAddressBody(edit func(addr map[string]any)) map[string]any {
	body := r.createBody()
	addr := map[string]any{
		"label": "Obra", "cep": "69057000", "street": "Rua Nova", "number": "5", "city": "Manaus", "state": "AM",
	}
	if edit != nil {
		edit(addr)
	}
	body["terms"] = map[string]any{"handover": "delivery", "newAddress": addr}
	return body
}

// The contract (checked by do) and the handler agree on terms.newAddress:
// order:write is enough, the address becomes the order's, and a quote with
// it answers without saving.
func TestOrders_NewAddressOnCreateAndQuote(t *testing.T) {
	t.Parallel()
	r := newVisRig(t)

	status, out := r.do(shared.RoleStaff, "POST", visBase+"/quote", map[string]any{
		"lines": r.createBody()["lines"], "terms": r.newAddressBody(nil)["terms"],
	}, "")
	if status != 200 || out["total"] == nil {
		t.Fatalf("quote with a new address answered %d %v", status, out)
	}
	status, out = r.do(shared.RoleStaff, "POST", visBase, r.newAddressBody(nil), r.key())
	if status != 201 {
		t.Fatalf("create with a new address answered %d %v", status, out)
	}
	addr, _ := orderOf(out)["address"].(map[string]any)
	if addr == nil || addr["street"] != "Rua Nova" || addr["id"] == nil {
		t.Errorf("order address = %v, want the new one with an id", addr)
	}
	if status, out = r.do(shared.RoleViewer, "POST", visBase, r.newAddressBody(nil), r.key()); status != 403 {
		t.Errorf("viewer answered %d %v, want 403", status, out)
	}
}

func TestOrders_NewAddressRefusalsNameTheirFields(t *testing.T) {
	t.Parallel()
	r := newVisRig(t)
	tests := []struct {
		name  string
		edit  func(map[string]any)
		field string
	}{
		{"bad cep", func(a map[string]any) { a["cep"] = "123" }, "terms.newAddress.cep"},
		{"bad state", func(a map[string]any) { a["state"] = "XX" }, "terms.newAddress.state"},
	}
	for _, tt := range tests {
		status, out := r.do(shared.RoleStaff, "POST", visBase, r.newAddressBody(tt.edit), r.key())
		if status != 400 || !hasIssue(out, tt.field) {
			t.Errorf("%s: answered %d %v, want 400 naming %s", tt.name, status, out, tt.field)
		}
	}
	both := r.newAddressBody(nil)
	both["terms"].(map[string]any)["addressId"] = shared.AddressID(shared.NewV7(1, [10]byte{9})).String()
	if status, out := r.do(shared.RoleStaff, "POST", visBase, both, r.key()); status != 400 || !hasIssue(out, "terms.newAddress") {
		t.Errorf("addressId and newAddress together: answered %d %v, want 400 on terms.newAddress", status, out)
	}
}

// hasIssue reports whether a problem body lists an error for field.
func hasIssue(problem map[string]any, field string) bool {
	issues, _ := problem["errors"].([]any)
	for _, i := range issues {
		if m, ok := i.(map[string]any); ok && m["field"] == field {
			return true
		}
	}
	return false
}
