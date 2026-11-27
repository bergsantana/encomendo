package customer_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/app/customer/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	start   = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	errBoom = errors.New("postgres is down")
)

const (
	phone  = "5592991234567"
	phone2 = "5511987654321"
)

type harness struct {
	svc    *appcustomer.Service
	store  *fake.Store
	tx     *sharedfake.TxManager
	ids    *sharedfake.IDs
	store1 shared.TenantID
	store2 shared.TenantID
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ids := sharedfake.NewIDs(sharedfake.NewClock(start))
	h := &harness{
		store: fake.NewStore(), tx: sharedfake.NewTxManager(), ids: ids,
		store1: shared.TenantID(ids.NewID()), store2: shared.TenantID(ids.NewID()),
	}
	h.svc = appcustomer.NewService(h.store, h.tx, ids)
	return h
}

var _ appcustomer.UseCases = (*appcustomer.Service)(nil)

// as returns a context of a staff user acting in tenant with role, as the
// authn middleware builds it.
func (h *harness) as(t *testing.T, tenant shared.TenantID, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(h.ids.NewID()), tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

func (h *harness) owner(t *testing.T) context.Context { return h.as(t, h.store1, shared.RoleOwner) }

func address(label string) customer.AddressSpec {
	return customer.AddressSpec{
		Label: label, CEP: "69000000", Street: "Rua das Flores", Number: "10",
		City: "Manaus", State: "AM",
	}
}

func spec(name, phone string, addresses ...customer.AddressSpec) customer.Spec {
	return customer.Spec{Name: name, Phone: phone, Email: "a@example.com", Notes: "Prefere à tarde.", Addresses: addresses}
}

// create adds a customer through the use case as the owner.
func (h *harness) create(t *testing.T, name, phone string, addresses ...customer.AddressSpec) appcustomer.View {
	t.Helper()
	v, err := h.svc.Create(h.owner(t), appcustomer.CreateInput{Spec: spec(name, phone, addresses...)})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	var out []string
	for _, f := range ve.Fields {
		out = append(out, f.Field)
	}
	return out
}

func addressIDs(v appcustomer.View) []shared.AddressID {
	out := make([]shared.AddressID, 0, len(v.Addresses))
	for _, a := range v.Addresses {
		out = append(out, a.ID)
	}
	return out
}

func TestPermissions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone)
	p := mustPage(t, "", 10)
	upd := appcustomer.UpdateInput{ID: v.ID, Spec: spec("Ana", phone), Version: 1}
	roles := []struct {
		role                              shared.Role
		canRead, canCreate, canUpdateWork bool
	}{
		{shared.RoleOwner, true, true, true},
		{shared.RoleAdmin, true, true, true},
		{shared.RoleStaff, true, true, false},
		{shared.RoleViewer, true, false, false},
	}
	for _, r := range roles {
		t.Run(string(r.role), func(t *testing.T) {
			ctx := h.as(t, h.store1, r.role)
			_, listErr := h.svc.List(ctx, appcustomer.ListFilter{}, p)
			_, getErr := h.svc.Get(ctx, v.ID)
			_, phoneErr := h.svc.ByPhone(ctx, phone)
			_, createErr := h.svc.Create(ctx, appcustomer.CreateInput{Spec: spec("Novo", phone2)})
			_, updateErr := h.svc.Update(ctx, upd)
			denied := func(err error) bool { return errors.Is(err, shared.ErrForbidden) }
			if denied(listErr) == r.canRead || denied(getErr) == r.canRead || denied(phoneErr) == r.canRead {
				t.Errorf("reads: list %v, get %v, by phone %v; allowed = %v", listErr, getErr, phoneErr, r.canRead)
			}
			if denied(createErr) == r.canCreate {
				t.Errorf("create: %v; allowed = %v", createErr, r.canCreate)
			}
			if denied(updateErr) == r.canUpdateWork {
				t.Errorf("update: %v; allowed = %v", updateErr, r.canUpdateWork)
			}
		})
	}
}

func TestPermissions_NoCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if _, err := h.svc.Get(context.Background(), shared.CustomerID(h.ids.NewID())); err == nil {
		t.Error("Get with no caller succeeded")
	}
	if got := h.tx.Scopes(); len(got) != 0 {
		t.Errorf("transactions opened before authorization: %v", got)
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)

	v, err := h.svc.Create(ctx, appcustomer.CreateInput{Spec: customer.Spec{
		Name: "  João Pereira ", Phone: phone, Email: "joao@example.com", Notes: " nota ",
		Addresses: []customer.AddressSpec{address("Casa"), address("Trabalho")},
	}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.ID.IsZero() || v.Name != "João Pereira" || v.Phone != phone || v.Email != "joao@example.com" ||
		v.Notes != "nota" || v.Version != 1 || len(v.Addresses) != 2 {
		t.Errorf("view = %+v, want the trimmed fields, two addresses and version 1", v)
	}
	if v.Addresses[0].ID.IsZero() || v.Addresses[0].ID == v.Addresses[1].ID || v.Addresses[0].Label != "Casa" {
		t.Errorf("addresses = %+v, want new distinct ids in order", v.Addresses)
	}
	got, err := h.svc.Get(ctx, v.ID)
	if err != nil || got.ID != v.ID || got.Name != v.Name || !slices.Equal(addressIDs(got), addressIDs(v)) {
		t.Errorf("Get = %+v, %v; want the created customer", got, err)
	}
	if scopes := h.tx.Scopes(); len(scopes) == 0 || scopes[0].TenantID != h.store1 {
		t.Errorf("scopes = %+v, want the transaction in the caller's store", scopes)
	}
}

func TestCreate_IgnoresSentAddressIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	sent := shared.AddressID(h.ids.NewID())
	a := address("Casa")
	a.ID = sent
	v := h.create(t, "Ana", phone, a)
	if v.Addresses[0].ID == sent {
		t.Error("Create kept an id the client chose; it must issue its own")
	}
}

func TestCreate_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	h.create(t, "Ana", phone)

	t.Run("invalid values, with every field named", func(t *testing.T) {
		_, err := h.svc.Create(ctx, appcustomer.CreateInput{Spec: spec(" ", "123")})
		if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "phone"}) {
			t.Errorf("fields = %v, want name and phone", got)
		}
	})
	t.Run("a number in use", func(t *testing.T) {
		_, err := h.svc.Create(ctx, appcustomer.CreateInput{Spec: spec("Outra Ana", phone)})
		if !errors.Is(err, appcustomer.ErrPhoneTaken) || !errors.Is(err, shared.ErrConflict) {
			t.Errorf("error = %v, want ErrPhoneTaken (a conflict)", err)
		}
	})
	t.Run("the same number in another store is fine", func(t *testing.T) {
		if _, err := h.svc.Create(h.as(t, h.store2, shared.RoleOwner), appcustomer.CreateInput{Spec: spec("Ana", phone)}); err != nil {
			t.Errorf("error = %v, want none: numbers are unique per store", err)
		}
	})
	t.Run("a failed insert", func(t *testing.T) {
		h.store.Fail("Insert", errBoom)
		defer h.store.Fail("Insert", nil)
		if _, err := h.svc.Create(ctx, appcustomer.CreateInput{Spec: spec("Bia", phone2)}); !errors.Is(err, errBoom) {
			t.Errorf("error = %v, want the repository's", err)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	v := h.create(t, "Ana", phone, address("Casa"), address("Trabalho"))
	home, work := v.Addresses[0], v.Addresses[1]

	keepHome := address("Casa nova")
	keepHome.ID = home.ID
	other := shared.AddressID(h.ids.NewID()) // an id the customer does not have
	added := address("Sítio")
	added.ID = other
	got, err := h.svc.Update(ctx, appcustomer.UpdateInput{
		ID: v.ID, Version: 1,
		Spec: customer.Spec{Name: "Ana Maria", Phone: phone2, Email: "", Notes: "", Addresses: []customer.AddressSpec{keepHome, added}},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "Ana Maria" || got.Phone != phone2 || got.Email != "" || got.Version != 2 || len(got.Addresses) != 2 {
		t.Errorf("view = %+v, want the new fields, no email, version 2 and two addresses", got)
	}
	if got.Addresses[0].ID != home.ID || got.Addresses[0].Label != "Casa nova" {
		t.Errorf("first address = %+v, want the kept id with its new label", got.Addresses[0])
	}
	if got.Addresses[1].ID == other || got.Addresses[1].ID == work.ID || got.Addresses[1].Label != "Sítio" {
		t.Errorf("second address = %+v, want a new id: an unknown id is never kept", got.Addresses[1])
	}
	read, err := h.svc.Get(ctx, v.ID)
	if err != nil || read.Version != 2 || !slices.Equal(addressIDs(read), addressIDs(got)) {
		t.Errorf("Get = %+v, %v; want the replaced customer at version 2", read, err)
	}
}

func TestUpdate_KeepsAddressesSent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone, address("Casa"))
	a := address("Casa")
	a.ID = v.Addresses[0].ID
	got, err := h.svc.Update(h.owner(t), appcustomer.UpdateInput{ID: v.ID, Version: 1, Spec: spec("Ana", phone, a)})
	if err != nil || !slices.Equal(addressIDs(got), addressIDs(v)) {
		t.Errorf("Update = %+v, %v; want the same address id", got, err)
	}
	cleared, err := h.svc.Update(h.owner(t), appcustomer.UpdateInput{ID: v.ID, Version: 2, Spec: spec("Ana", phone)})
	if err != nil || len(cleared.Addresses) != 0 {
		t.Errorf("Update with no addresses = %+v, %v; want them all removed", cleared, err)
	}
}

func TestUpdate_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	v := h.create(t, "Ana", phone)
	h.create(t, "Bia", phone2)
	if _, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 1, Spec: spec("Ana", phone)}); err != nil {
		t.Fatal(err)
	}

	t.Run("a stale version", func(t *testing.T) {
		_, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 1, Spec: spec("Ana Nova", phone)})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("error = %v, want ErrVersionConflict", err)
		}
		if read, _ := h.svc.Get(ctx, v.ID); read.Name != "Ana" || read.Version != 2 {
			t.Errorf("stored = %+v, want it unchanged", read)
		}
	})
	t.Run("a stale version wins over invalid values", func(t *testing.T) {
		_, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 1, Spec: spec("", "1")})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("error = %v, want ErrVersionConflict", err)
		}
	})
	t.Run("invalid values, with every field named", func(t *testing.T) {
		_, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 2, Spec: spec(" ", "1")})
		if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "phone"}) {
			t.Errorf("fields = %v, want name and phone", got)
		}
	})
	t.Run("a number in use", func(t *testing.T) {
		_, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 2, Spec: spec("Ana", phone2)})
		if !errors.Is(err, appcustomer.ErrPhoneTaken) {
			t.Errorf("error = %v, want ErrPhoneTaken", err)
		}
		if read, _ := h.svc.Get(ctx, v.ID); read.Phone != phone || read.Version != 2 {
			t.Errorf("stored = %+v, want it unchanged", read)
		}
	})
	t.Run("an unknown customer", func(t *testing.T) {
		_, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: shared.CustomerID(h.ids.NewID()), Version: 1, Spec: spec("X", phone)})
		if !errors.Is(err, customer.ErrNotFound) || !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error = %v, want customer.ErrNotFound", err)
		}
	})
	t.Run("another store's customer", func(t *testing.T) {
		_, err := h.svc.Update(h.as(t, h.store2, shared.RoleOwner), appcustomer.UpdateInput{ID: v.ID, Version: 2, Spec: spec("X", phone)})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error = %v, want not found", err)
		}
	})
	t.Run("a failed save", func(t *testing.T) {
		h.store.Fail("Update", errBoom)
		defer h.store.Fail("Update", nil)
		if _, err := h.svc.Update(ctx, appcustomer.UpdateInput{ID: v.ID, Version: 2, Spec: spec("Ana", phone)}); !errors.Is(err, errBoom) {
			t.Errorf("error = %v, want the repository's", err)
		}
	})
}

func TestGet_And_ByPhone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	v := h.create(t, "Ana", phone, address("Casa"))

	got, err := h.svc.ByPhone(ctx, phone)
	if err != nil || got.ID != v.ID || len(got.Addresses) != 1 {
		t.Errorf("ByPhone = %+v, %v; want the customer", got, err)
	}
	if _, err := h.svc.ByPhone(ctx, phone2); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("ByPhone of an unknown number: %v, want ErrNotFound", err)
	}
	other := h.as(t, h.store2, shared.RoleOwner)
	if _, err := h.svc.Get(other, v.ID); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("Get of another store's customer: %v, want ErrNotFound", err)
	}
	if _, err := h.svc.ByPhone(other, phone); !errors.Is(err, customer.ErrNotFound) {
		t.Errorf("ByPhone of another store's customer: %v, want ErrNotFound", err)
	}
	h.store.Fail("ByID", errBoom)
	if _, err := h.svc.Get(ctx, v.ID); !errors.Is(err, errBoom) {
		t.Errorf("Get with a failing repository: %v, want its error", err)
	}
	h.store.Fail("ByID", nil)
	h.store.Fail("ByPhone", errBoom)
	if _, err := h.svc.ByPhone(ctx, phone); !errors.Is(err, errBoom) {
		t.Errorf("ByPhone with a failing repository: %v, want its error", err)
	}
}

func names(p shared.Page[appcustomer.Summary]) []string {
	out := make([]string, 0, len(p.Items))
	for _, s := range p.Items {
		out = append(out, s.Name)
	}
	return out
}

func TestList(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	ana := h.create(t, "ana Souza", "5592991111111")
	h.create(t, "Bruno", "5592992222222")
	h.create(t, "Anabela", "5511993333333")
	h.create(t, "Carla", "5511994444444")
	h.store.AddOrder(ana.ID, start.Add(-48*time.Hour))
	h.store.AddOrder(ana.ID, start.Add(-24*time.Hour))

	page, err := h.svc.List(ctx, appcustomer.ListFilter{}, mustPage(t, "", 3))
	if err != nil || !slices.Equal(names(page), []string{"ana Souza", "Anabela", "Bruno"}) {
		t.Fatalf("first page = %v, %v", names(page), err)
	}
	if !page.HasMore() {
		t.Error("the first page of 3 of 4 has no cursor")
	}
	rest, err := h.svc.List(ctx, appcustomer.ListFilter{}, mustPage(t, page.NextCursor, 3))
	if err != nil || len(rest.Items) != 1 || rest.Items[0].Name != "Carla" || rest.HasMore() {
		t.Errorf("second page = %v, %v; want Carla alone", names(rest), err)
	}

	t.Run("a name prefix, ignoring case", func(t *testing.T) {
		p, err := h.svc.List(ctx, appcustomer.ListFilter{Query: " ANA"}, mustPage(t, "", 10))
		if err != nil || !slices.Equal(names(p), []string{"ana Souza", "Anabela"}) {
			t.Errorf("filter ANA = %v, %v; want ana Souza and Anabela", names(p), err)
		}
	})
	t.Run("digits of the phone", func(t *testing.T) {
		p, err := h.svc.List(ctx, appcustomer.ListFilter{Query: "(92) 99222"}, mustPage(t, "", 10))
		if err != nil || !slices.Equal(names(p), []string{"Bruno"}) {
			t.Errorf("filter by digits = %v, %v; want Bruno", names(p), err)
		}
	})
	t.Run("a name with a digit is not a phone", func(t *testing.T) {
		p, err := h.svc.List(ctx, appcustomer.ListFilter{Query: "Ana 9"}, mustPage(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("filter %q = %v, %v; want none", "Ana 9", names(p), err)
		}
	})
	t.Run("order count and last order", func(t *testing.T) {
		p, err := h.svc.List(ctx, appcustomer.ListFilter{Query: "ana s"}, mustPage(t, "", 10))
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("filter = %v, %v", names(p), err)
		}
		s := p.Items[0]
		if s.ID != ana.ID || s.Orders != 2 || s.LastOrderAt == nil || !s.LastOrderAt.Equal(start.Add(-24*time.Hour)) {
			t.Errorf("summary = %+v, want 2 orders, the newest a day ago", s)
		}
		none, _ := h.svc.List(ctx, appcustomer.ListFilter{Query: "bruno"}, mustPage(t, "", 10))
		if none.Items[0].Orders != 0 || none.Items[0].LastOrderAt != nil {
			t.Errorf("summary = %+v, want no orders and no date", none.Items[0])
		}
	})
	t.Run("another store sees none", func(t *testing.T) {
		p, err := h.svc.List(h.as(t, h.store2, shared.RoleViewer), appcustomer.ListFilter{}, mustPage(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("list = %v, %v; want none", names(p), err)
		}
	})
	t.Run("a bad cursor", func(t *testing.T) {
		_, err := h.svc.List(ctx, appcustomer.ListFilter{}, mustPage(t, "not-a-cursor", 10))
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("error = %v, want a ValidationError", err)
		}
	})
	t.Run("a failing repository", func(t *testing.T) {
		h.store.Fail("List", errBoom)
		defer h.store.Fail("List", nil)
		if _, err := h.svc.List(ctx, appcustomer.ListFilter{}, mustPage(t, "", 10)); !errors.Is(err, errBoom) {
			t.Errorf("error = %v, want the repository's", err)
		}
	})
}

func TestListFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		query, prefix, digits string
	}{
		{"", "", ""},
		{"  Ana ", "ana", ""},
		{"92 99123-4567", "92 99123-4567", "92991234567"},
		{"+55 (92) 9", "+55 (92) 9", "55929"},
		{"Ana 2", "ana 2", ""},
		{"()", "()", ""},
	}
	for _, tt := range tests {
		f := appcustomer.ListFilter{Query: tt.query}
		if got := f.NamePrefix(); got != tt.prefix {
			t.Errorf("NamePrefix(%q) = %q, want %q", tt.query, got, tt.prefix)
		}
		if got := f.PhoneDigits(); got != tt.digits {
			t.Errorf("PhoneDigits(%q) = %q, want %q", tt.query, got, tt.digits)
		}
	}
}

func TestWritesRollBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Ana", phone)
	// A write that fails inside the transaction leaves no trace in the store.
	h.store.Fail("Update", errBoom)
	_, _ = h.svc.Update(h.owner(t), appcustomer.UpdateInput{ID: v.ID, Version: 1, Spec: spec("Outra", phone)})
	h.store.Fail("Update", nil)
	if read, _ := h.svc.Get(h.owner(t), v.ID); read.Name != "Ana" || read.Version != 1 {
		t.Errorf("stored = %+v, want it unchanged", read)
	}
}
