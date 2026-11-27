package tenancy

import (
	"context"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// StoreDefaultsView is the part of a store's settings the catalog builds on:
// the currency of its prices, the default assembly days and safety margin of
// its kits, and which sourcing modes are on. The store page may compute
// with these values (a kit's total days), but never shows the safety margin
// on its own: it is the store's buffer. Orders build on the rest: the
// deposit percent, the time zone that decides "today", the store's name,
// address and WhatsApp number for the wa.me messages, and the status
// workflow new orders take.
type StoreDefaultsView struct {
	Currency          string
	AssemblyDays      int
	SafetyMarginDays  int
	StockEnabled      bool
	ProductionEnabled bool
	DepositPercent    int
	TimeZone          tenancy.TimeZone
	Slug              string
	StoreName         string
	WhatsAppPhone     string
	Workflow          tenancy.WorkflowRef
}

// StoreDefaults returns the defaults of the store in the request's scope. It
// needs only a tenant in the scope, not a principal, because the store page
// (a tenant and no user) prices kits with these days too; it is not on
// UseCases, and adapter/inproc wraps it for the catalog. It returns
// authz.ErrTenantRequired when the scope has no tenant, such as a client
// area request, without starting a transaction. Every store has settings, so
// a missing row is a bug (500).
func (s *Service) StoreDefaults(ctx context.Context) (StoreDefaultsView, error) {
	tenant := appshared.ScopeFrom(ctx).TenantID
	if tenant.IsZero() {
		return StoreDefaultsView{}, authz.ErrTenantRequired
	}
	var st *tenancy.Settings
	var store *tenancy.Tenant
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		read, err := s.tenants.SettingsOf(ctx, tenant)
		if err != nil {
			return mustExist("read store defaults", err)
		}
		t, err := s.tenants.TenantByID(ctx, tenant)
		if err != nil {
			return mustExist("read store", err)
		}
		st, store = read, t
		return nil
	})
	if err != nil {
		return StoreDefaultsView{}, err
	}
	e := st.Editable()
	return StoreDefaultsView{
		Currency: st.Currency(), AssemblyDays: e.AssemblyDays, SafetyMarginDays: e.SafetyMarginDays,
		StockEnabled: e.StockEnabled, ProductionEnabled: e.ProductionEnabled,
		DepositPercent: e.DepositPercent, TimeZone: e.TimeZone, Slug: store.Slug().String(), StoreName: store.Name().String(),
		WhatsAppPhone: e.WhatsAppPhone, Workflow: st.Workflow(),
	}, nil
}
