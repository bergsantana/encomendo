package tenancy

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The ranges of docs/api/openapi.yaml (EditableSettings) and the CHECK
// constraints of 0004_tenancy.
const (
	maxDepositPercent = 100
	maxDays           = 365
	defaultCurrency   = "BRL"
)

// WorkflowRef names the status workflow new orders take (ADR 0004): one
// immutable published version of a preset. Name is the preset's pt-BR name,
// carried for display; rules use ID and Version only.
type WorkflowRef struct {
	ID      shared.WorkflowID
	Version int
	Name    string
}

// IsZero reports whether no workflow is set. A store never has none (ADR
// 0006).
func (w WorkflowRef) IsZero() bool { return w.ID.IsZero() || w.Version < 1 }

// Editable is what the owner changes in settings. The workflow is chosen
// elsewhere (milestone 4) and the currency is fixed.
type Editable struct {
	StockEnabled          bool // Estoque and the stock sourcing mode
	ProductionEnabled     bool // Produção and the in-house sourcing mode
	PurchaseBeforeDeposit bool // purchases before the Sinal is paid
	DepositPercent        int
	AssemblyDays          int
	SafetyMarginDays      int    // Margem de segurança
	WhatsAppPhone         string // 55 + DDD + number, digits only; "" for none
}

// Validate returns a ValidationError listing every field out of range: the
// deposit 0 to 100, assembly days and safety margin 0 to 365, and the
// WhatsApp number as wa.me needs it.
func (e Editable) Validate() error {
	var fields []shared.FieldError
	if e.DepositPercent < 0 || e.DepositPercent > maxDepositPercent {
		fields = append(fields, shared.FieldError{Field: "depositPercent", Message: "Use um sinal de 0 a 100%."})
	}
	if e.AssemblyDays < 0 || e.AssemblyDays > maxDays {
		fields = append(fields, shared.FieldError{Field: "assemblyDays", Message: "Use de 0 a 365 dias."})
	}
	if e.SafetyMarginDays < 0 || e.SafetyMarginDays > maxDays {
		fields = append(fields, shared.FieldError{Field: "safetyMarginDays", Message: "Use de 0 a 365 dias."})
	}
	if e.WhatsAppPhone != "" && !whatsAppPhone(e.WhatsAppPhone) {
		fields = append(fields, shared.FieldError{
			Field:   "whatsappPhone",
			Message: "Use 55, o DDD e o número, só com dígitos. Exemplo: 5592991234567.",
		})
	}
	if len(fields) > 0 {
		return shared.NewValidationError(fields...)
	}
	return nil
}

// whatsAppPhone matches ^55[1-9]{2}[0-9]{8,9}$.
func whatsAppPhone(p string) bool {
	if len(p) != 12 && len(p) != 13 || p[0] != '5' || p[1] != '5' || p[2] == '0' || p[3] == '0' {
		return false
	}
	for i := range len(p) {
		if p[i] < '0' || p[i] > '9' {
			return false
		}
	}
	return true
}

// Settings are one store's defaults and switches.
type Settings struct {
	tenant   shared.TenantID
	workflow WorkflowRef
	editable Editable
	currency string
	version  int
}

// DefaultSettings returns a new store's settings (docs/architecture.md 4.2):
// deposit 50%, assembly 3 days, safety margin 2 days, BRL, stock and
// production off, purchases only after the deposit, and workflow as the
// status workflow. It returns ErrInvalidSettings for a zero tenant or
// workflow.
func DefaultSettings(tenant shared.TenantID, workflow WorkflowRef) (*Settings, error) {
	if tenant.IsZero() || workflow.IsZero() {
		return nil, fmt.Errorf("default settings of %v: %w", tenant, ErrInvalidSettings)
	}
	return &Settings{
		tenant:   tenant,
		workflow: workflow,
		editable: Editable{DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2},
		currency: defaultCurrency,
		version:  1,
	}, nil
}

// SettingsRecord is a store's settings as a repository stores them.
type SettingsRecord struct {
	Tenant   shared.TenantID
	Workflow WorkflowRef
	Editable Editable
	Currency string
	Version  int
}

// RestoreSettings rebuilds stored settings. It returns ErrInvalidSettings
// when the record breaks a rule stored settings always keep.
func RestoreSettings(r SettingsRecord) (*Settings, error) {
	if r.Tenant.IsZero() || r.Workflow.IsZero() || !currencyCode(r.Currency) || r.Version < 1 || r.Editable.Validate() != nil {
		return nil, fmt.Errorf("restore settings of %v: %w", r.Tenant, ErrInvalidSettings)
	}
	return &Settings{tenant: r.Tenant, workflow: r.Workflow, editable: r.Editable, currency: r.Currency, version: r.Version}, nil
}

func currencyCode(c string) bool {
	return len(c) == 3 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' && c[2] >= 'A' && c[2] <= 'Z'
}

// TenantID returns the store.
func (s *Settings) TenantID() shared.TenantID { return s.tenant }

// Workflow returns the status workflow new orders take.
func (s *Settings) Workflow() WorkflowRef { return s.workflow }

// Editable returns what the owner can change.
func (s *Settings) Editable() Editable { return s.editable }

// Currency returns the store's currency, fixed at BRL for now.
func (s *Settings) Currency() string { return s.currency }

// Version returns the version the settings were read at. A repository's
// update stores the next one, and only while the stored version is still
// this one.
func (s *Settings) Version() int { return s.version }

// Change replaces every editable setting with e, as the owner sends them
// all with the version they read. It returns a ValidationError when e is out
// of range and shared.ErrVersionConflict when expectedVersion is not the
// version read; either way nothing changes.
func (s *Settings) Change(e Editable, expectedVersion int) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if expectedVersion != s.version {
		return shared.ErrVersionConflict
	}
	s.editable = e
	return nil
}
