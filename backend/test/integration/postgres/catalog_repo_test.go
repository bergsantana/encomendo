//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// Cases the port contracts leave out: what only Postgres can show (jsonb,
// photo rows, the cascade, constraint names).

type catalogEnv struct {
	t      *testing.T
	seed   *pgtest.Seeder
	tx     *postgres.TxManager
	tenant shared.TenantID
	scope  appshared.Scope
}

func newCatalogEnv(t *testing.T) catalogEnv {
	t.Helper()
	s := env.Seeder(t)
	tenant := s.Tenant(s.User())
	return catalogEnv{
		t: t, seed: s, tx: postgres.NewTxManager(openApp(t)), tenant: tenant,
		scope: appshared.Scope{TenantID: tenant, UserID: shared.UserID(pgtest.NewID())},
	}
}

func (e catalogEnv) run(fn func(ctx context.Context) error) error {
	return e.tx.WithinTx(appshared.WithScope(context.Background(), e.scope), fn)
}

func (e catalogEnv) must(fn func(ctx context.Context) error) {
	e.t.Helper()
	if err := e.run(fn); err != nil {
		e.t.Fatal(err)
	}
}

func brl(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (e catalogEnv) inHouseItem(name string, options []catalog.OptionInput) *catalog.Item {
	e.t.Helper()
	it, err := catalog.NewItem(e.tenant, shared.ItemID(pgtest.NewID()), catalog.ItemInput{
		ItemDetails: catalog.ItemDetails{Name: name, Price: brl(e.t, 10000), Options: options},
		Sourcing:    catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 4, ProductionCost: catalog.SetCost(brl(e.t, 900))},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return it
}

func TestItemsRepository_OptionsRoundTripThroughJSONB(t *testing.T) {
	e := newCatalogEnv(t)
	options := []catalog.OptionInput{
		{ID: shared.OptionID(pgtest.NewID()), Name: "Cor \"da\" costura", Required: true, Choices: []catalog.ChoiceInput{
			{ID: shared.ChoiceID(pgtest.NewID()), Label: "Vermelha çã", ExtraPrice: brl(t, 500), ExtraDays: 2},
			{ID: shared.ChoiceID(pgtest.NewID()), Label: "Preta", ExtraPrice: brl(t, 0), ExtraDays: 0},
		}},
		{ID: shared.OptionID(pgtest.NewID()), Name: "Logo", Choices: []catalog.ChoiceInput{
			{ID: shared.ChoiceID(pgtest.NewID()), Label: "Bordado", ExtraPrice: brl(t, 1250), ExtraDays: 365},
		}},
	}
	it := e.inHouseItem("Banco", options)
	e.must(func(ctx context.Context) error { return postgres.NewItems().Insert(ctx, it) })

	var raw string
	row := e.seed.DB().QueryRowContext(t.Context(),
		`SELECT choices::text FROM customization_options WHERE id = $1`, options[0].ID.String())
	if err := row.Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if want := `"extraPriceCents": 500`; !strings.Contains(raw, want) {
		t.Errorf("stored choices = %s, want the shape of ADR 0024 (%s)", raw, want)
	}
	e.must(func(ctx context.Context) error {
		got, err := postgres.NewItems().ByID(ctx, e.tenant, it.ID())
		if err != nil {
			return err
		}
		gotOpts, want := got.Options(), it.Options()
		if len(gotOpts) != 2 {
			t.Fatalf("options = %d, want 2", len(gotOpts))
		}
		for i := range want {
			if gotOpts[i].ID() != want[i].ID() || gotOpts[i].Name() != want[i].Name() || gotOpts[i].Required() != want[i].Required() ||
				!slices.Equal(gotOpts[i].Choices(), want[i].Choices()) {
				t.Errorf("option %d read back as %+v, want %+v", i, gotOpts[i], want[i])
			}
		}
		return nil
	})
}

func TestItemsRepository_ReadsPhotosAndNeverWritesThem(t *testing.T) {
	e := newCatalogEnv(t)
	items := postgres.NewItems()
	it := e.inHouseItem("Banco", nil)
	e.must(func(ctx context.Context) error { return items.Insert(ctx, it) })
	photoID := pgtest.NewID()
	key := "t/" + e.tenant.String() + "/items/" + it.ID().String() + "/" + photoID.String()
	if _, err := e.seed.DB().ExecContext(t.Context(),
		`INSERT INTO photos (tenant_id, id, item_id, object_key, content_type, size_bytes, position) VALUES ($1, $2, $3, $4, 'image/png', 1234, 0)`,
		e.tenant.String(), photoID.String(), it.ID().String(), key); err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error {
		got, err := items.ByID(ctx, e.tenant, it.ID())
		if err != nil {
			return err
		}
		photos := got.Photos()
		if len(photos) != 1 || photos[0].Key() != key || photos[0].Size() != 1234 || photos[0].ContentType() != "image/png" {
			t.Fatalf("photos = %+v, want the stored one", photos)
		}
		if err := got.Edit(catalog.ItemDetails{Name: "Banco novo", Price: got.Price()}); err != nil {
			return err
		}
		return items.Update(ctx, got)
	})
	var n int
	if err := e.seed.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM photos WHERE item_id = $1`, it.ID().String()).Scan(&n); err != nil || n != 1 {
		t.Errorf("photo rows after an update = %d, %v; want the row kept", n, err)
	}
}

func TestItemsRepository_ArchivedKeepsItsDate(t *testing.T) {
	e := newCatalogEnv(t)
	items := postgres.NewItems()
	it := e.inHouseItem("Banco", nil)
	e.must(func(ctx context.Context) error { return items.Insert(ctx, it) })
	archive := func(version int) {
		e.must(func(ctx context.Context) error {
			got, err := items.ByID(ctx, e.tenant, it.ID())
			if err != nil {
				return err
			}
			if got.Version() != version {
				t.Fatalf("version = %d, want %d", got.Version(), version)
			}
			got.Archive()
			return items.Update(ctx, got)
		})
	}
	stamp := func() (s string) {
		if err := e.seed.DB().QueryRowContext(t.Context(), `SELECT archived_at::text FROM items WHERE id = $1`, it.ID().String()).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	archive(1)
	first := stamp()
	e.must(func(ctx context.Context) error {
		got, err := items.ByID(ctx, e.tenant, it.ID())
		if err != nil {
			return err
		}
		got.Restore()
		return items.Update(ctx, got)
	})
	var restored *string
	if err := e.seed.DB().QueryRowContext(t.Context(), `SELECT archived_at::text FROM items WHERE id = $1`, it.ID().String()).Scan(&restored); err != nil || restored != nil {
		t.Errorf("archived_at after a restore = %v, %v; want null", restored, err)
	}
	archive(3)
	if second := stamp(); second == "" || first == "" {
		t.Errorf("archived_at = %q then %q, want a date each time", first, second)
	}
}

func TestCategoriesRepository_InUseAndNameMappings(t *testing.T) {
	e := newCatalogEnv(t)
	cats := postgres.NewCategories()
	c, err := catalog.NewCategory(e.tenant, shared.CategoryID(pgtest.NewID()), "Bancos", 0)
	if err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return cats.Insert(ctx, c) })
	it, err := catalog.NewItem(e.tenant, shared.ItemID(pgtest.NewID()), catalog.ItemInput{
		ItemDetails: catalog.ItemDetails{Name: "Banco", Price: brl(t, 100), Category: c.ID()},
		Sourcing:    catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return postgres.NewItems().Insert(ctx, it) })

	err = e.run(func(ctx context.Context) error { return cats.Delete(ctx, e.tenant, c.ID()) })
	if !errors.Is(err, catalog.ErrCategoryInUse) || !errors.Is(err, shared.ErrConflict) {
		t.Errorf("deleting a category in use: %v, want ErrCategoryInUse", err)
	}
	// The same id is a conflict, not a taken name.
	same, _ := catalog.NewCategory(e.tenant, c.ID(), "Outro", 1)
	err = e.run(func(ctx context.Context) error { return cats.Insert(ctx, same) })
	if !errors.Is(err, shared.ErrConflict) || errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("a taken id: %v, want ErrConflict and not ErrNameTaken", err)
	}
}

func TestAttributesRepository_UpdateKeepsRenamesAndAddsValues(t *testing.T) {
	e := newCatalogEnv(t)
	attrs := postgres.NewAttributes()
	ids := []shared.AttributeValueID{shared.AttributeValueID(pgtest.NewID()), shared.AttributeValueID(pgtest.NewID())}
	a, err := catalog.NewAttribute(e.tenant, shared.AttributeID(pgtest.NewID()), catalog.AttributeInput{
		Name: "Serve em", Values: []catalog.ValueInput{{ID: ids[0], Label: "CB 500X"}, {ID: ids[1], Label: "XRE 300"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.must(func(ctx context.Context) error { return attrs.Insert(ctx, a, 0) })
	added := shared.AttributeValueID(pgtest.NewID())
	e.must(func(ctx context.Context) error {
		cur, err := attrs.ByID(ctx, e.tenant, a.ID())
		if err != nil {
			return err
		}
		removed, err := cur.Attribute.Replace(catalog.AttributeInput{Name: "Serve em", Values: []catalog.ValueInput{
			{ID: added, Label: "Nova"}, {ID: ids[1], Label: "XRE 300 ABS"},
		}})
		if err != nil {
			return err
		}
		if err := attrs.Update(ctx, cur.Attribute, removed); err != nil {
			return err
		}
		got, err := attrs.ByID(ctx, e.tenant, a.ID())
		if err != nil {
			return err
		}
		vals := got.Attribute.Values()
		if len(vals) != 2 || vals[0].ID() != added || vals[1].ID() != ids[1] || vals[1].Label() != "XRE 300 ABS" || got.Attribute.Version() != 2 {
			t.Errorf("values after the update = %+v at version %d; want the new value first, the kept one renamed", vals, got.Attribute.Version())
		}
		return nil
	})
	// A value id some other field already has is a conflict.
	other, err := catalog.NewAttribute(e.tenant, shared.AttributeID(pgtest.NewID()), catalog.AttributeInput{
		Name: "Material", Values: []catalog.ValueInput{{ID: ids[1], Label: "Couro"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.run(func(ctx context.Context) error { return attrs.Insert(ctx, other, 1) })
	if !errors.Is(err, shared.ErrConflict) || errors.Is(err, catalog.ErrNameTaken) {
		t.Errorf("a value id in use: %v, want ErrConflict and not ErrNameTaken", err)
	}
}

func TestItemsRepository_CursorSurvivesTheRowLeavingTheFilter(t *testing.T) {
	e := newCatalogEnv(t)
	items := postgres.NewItems()
	for _, n := range []string{"Alfa", "Bravo", "Charlie"} {
		it := e.inHouseItem(n, nil)
		e.must(func(ctx context.Context) error { return items.Insert(ctx, it) })
	}
	e.must(func(ctx context.Context) error {
		first, err := items.List(ctx, e.tenant, appcatalog.ItemFilter{}, mustPage(t, "", 2))
		if err != nil || len(first.Items) != 2 || !first.HasMore() {
			t.Fatalf("first page = %v, %v", first.Items, err)
		}
		last := first.Items[1]
		last.Archive()
		if err := items.Update(ctx, last); err != nil {
			return err
		}
		rest, err := items.List(ctx, e.tenant, appcatalog.ItemFilter{}, mustPage(t, first.NextCursor, 2))
		if err != nil || len(rest.Items) != 1 || rest.Items[0].Name() != "Charlie" {
			t.Errorf("page after the archived row = %v, %v; want Charlie", rest.Items, err)
		}
		return nil
	})
}

func TestItemsRepository_QueryIsLiteralWithBackslashes(t *testing.T) {
	e := newCatalogEnv(t)
	items := postgres.NewItems()
	for _, n := range []string{`A\B`, "AxB", "100% puro"} {
		it := e.inHouseItem(n, nil)
		e.must(func(ctx context.Context) error { return items.Insert(ctx, it) })
	}
	e.must(func(ctx context.Context) error {
		for q, want := range map[string]string{`\`: `A\B`, `A\B`: `A\B`, "%": "100% puro"} {
			p, err := items.List(ctx, e.tenant, appcatalog.ItemFilter{Query: q}, mustPage(t, "", 10))
			if err != nil || len(p.Items) != 1 || p.Items[0].Name() != want {
				t.Errorf("query %q = %v, %v; want only %q", q, p.Items, err, want)
			}
		}
		return nil
	})
}

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
