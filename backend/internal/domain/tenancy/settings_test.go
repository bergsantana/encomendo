package tenancy

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var defaultWorkflow = WorkflowRef{
	ID:      shared.WorkflowID(shared.NewV7(t0.UnixMilli(), [10]byte{5})),
	Version: 1,
	Name:    "Encomenda com montagem",
}

func TestDefaultSettings(t *testing.T) {
	t.Parallel()

	s, err := DefaultSettings(tenantID, defaultWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	// docs/architecture.md 4.2: deposit 50%, assembly 3 days, safety margin
	// 2 days; stock and production off; purchases only after the deposit.
	want := Editable{DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, TimeZone: ZoneSaoPaulo}
	if s.Editable() != want {
		t.Errorf("defaults = %+v, want %+v", s.Editable(), want)
	}
	if s.Currency() != "BRL" || s.Workflow() != defaultWorkflow || s.Version() != 1 || s.TenantID() != tenantID {
		t.Errorf("currency %q, workflow %+v, version %d", s.Currency(), s.Workflow(), s.Version())
	}
	if _, err := DefaultSettings(shared.TenantID{}, defaultWorkflow); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("zero tenant: %v", err)
	}
	if _, err := DefaultSettings(tenantID, WorkflowRef{}); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("no workflow: %v", err)
	}
}

func TestEditable_Validate(t *testing.T) {
	t.Parallel()

	ok := []Editable{
		{DepositPercent: 0, AssemblyDays: 0, SafetyMarginDays: 0, TimeZone: ZoneNoronha},
		{DepositPercent: 100, AssemblyDays: 365, SafetyMarginDays: 365, WhatsAppPhone: "5592991234567", TimeZone: ZoneRioBranco},
		{DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, WhatsAppPhone: "551133334444", TimeZone: ZoneManaus},
	}
	for _, e := range ok {
		if err := e.Validate(); err != nil {
			t.Errorf("%+v: %v", e, err)
		}
	}

	bad := Editable{DepositPercent: 101, AssemblyDays: -1, SafetyMarginDays: 366, WhatsAppPhone: "(92) 99123-4567", TimeZone: "UTC"}
	err := bad.Validate()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate = %v, want a ValidationError", err)
	}
	var fields []string
	for _, f := range ve.Fields {
		fields = append(fields, f.Field)
		if f.Message == "" {
			t.Errorf("%s has no message", f.Field)
		}
	}
	want := []string{"depositPercent", "assemblyDays", "safetyMarginDays", "whatsappPhone", "timeZone"}
	if len(fields) != len(want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("fields = %v, want %v", fields, want)
		}
	}
}

func TestEditable_ValidatePhone(t *testing.T) {
	t.Parallel()

	// wa.me needs 55 + DDD + number, digits only (docs/api/openapi.yaml).
	for phone, ok := range map[string]bool{
		"":                 true, // none
		"5592991234567":    true,
		"559291234567":     true,
		"92991234567":      false, // no country code
		"5502991234567":    false, // DDD cannot start with 0
		"55929912345678":   false, // too long
		"+5592991234567":   false,
		"55 92 99123-4567": false,
		"559299123x567":    false, // the right length, not all digits
		"5590991234567":    false, // the DDD's second digit cannot be 0
	} {
		err := Editable{WhatsAppPhone: phone, TimeZone: ZoneSaoPaulo}.Validate()
		if (err == nil) != ok {
			t.Errorf("phone %q: err = %v, want ok = %v", phone, err, ok)
		}
	}
}

func TestSettings_Change(t *testing.T) {
	t.Parallel()

	s, _ := RestoreSettings(SettingsRecord{
		Tenant: tenantID, Workflow: defaultWorkflow, Currency: "BRL", Version: 4,
		Editable: Editable{DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, TimeZone: ZoneSaoPaulo},
	})
	next := Editable{StockEnabled: true, DepositPercent: 30, AssemblyDays: 5, SafetyMarginDays: 1, WhatsAppPhone: "5592991234567", TimeZone: ZoneManaus}

	if err := s.Change(next, 3); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("stale version: %v, want ErrVersionConflict", err)
	}
	if s.Editable().DepositPercent != 50 {
		t.Error("a refused change changed the settings")
	}
	if err := s.Change(Editable{DepositPercent: 200, TimeZone: ZoneSaoPaulo}, 4); !errors.Is(err, shared.ErrInvalid) || s.Editable().DepositPercent != 50 {
		t.Errorf("invalid change: %v; settings %+v", err, s.Editable())
	}
	if err := s.Change(next, 4); err != nil {
		t.Fatal(err)
	}
	if s.Editable() != next {
		t.Errorf("after Change = %+v, want %+v", s.Editable(), next)
	}
	// The version stays the one read; the repository stores the next.
	if s.Version() != 4 || s.Currency() != "BRL" || s.Workflow() != defaultWorkflow {
		t.Errorf("Change touched version %d, currency %q or workflow %+v", s.Version(), s.Currency(), s.Workflow())
	}
}

func TestSettings_ChooseWorkflow(t *testing.T) {
	t.Parallel()

	other := WorkflowRef{ID: shared.WorkflowID(shared.NewV7(t0.UnixMilli(), [10]byte{6})), Version: 2, Name: "Produção própria"}
	newSettings := func(t *testing.T) *Settings {
		t.Helper()
		s, err := RestoreSettings(SettingsRecord{
			Tenant: tenantID, Workflow: defaultWorkflow, Currency: "BRL", Version: 4,
			Editable: Editable{DepositPercent: 50, TimeZone: ZoneSaoPaulo},
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("replaces the workflow and keeps the version", func(t *testing.T) {
		t.Parallel()
		s := newSettings(t)
		if err := s.ChooseWorkflow(other, 4); err != nil {
			t.Fatal(err)
		}
		if s.Workflow() != other || s.Version() != 4 {
			t.Errorf("workflow %+v, version %d", s.Workflow(), s.Version())
		}
	})
	t.Run("a stale version changes nothing", func(t *testing.T) {
		t.Parallel()
		s := newSettings(t)
		if err := s.ChooseWorkflow(other, 3); !errors.Is(err, shared.ErrVersionConflict) || s.Workflow() != defaultWorkflow {
			t.Errorf("err = %v, workflow %+v", err, s.Workflow())
		}
	})
	t.Run("a zero reference is refused", func(t *testing.T) {
		t.Parallel()
		s := newSettings(t)
		if err := s.ChooseWorkflow(WorkflowRef{}, 4); !errors.Is(err, ErrInvalidSettings) || s.Workflow() != defaultWorkflow {
			t.Errorf("err = %v, workflow %+v", err, s.Workflow())
		}
	})
}

func TestRestoreSettings_Refuses(t *testing.T) {
	t.Parallel()

	good := SettingsRecord{Tenant: tenantID, Workflow: defaultWorkflow, Currency: "BRL", Version: 1, Editable: Editable{DepositPercent: 50, TimeZone: ZoneSaoPaulo}}
	if _, err := RestoreSettings(good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*SettingsRecord){
		"zero tenant":  func(r *SettingsRecord) { r.Tenant = shared.TenantID{} },
		"no workflow":  func(r *SettingsRecord) { r.Workflow = WorkflowRef{} },
		"bad currency": func(r *SettingsRecord) { r.Currency = "real" },
		"zero version": func(r *SettingsRecord) { r.Version = 0 },
		"bad deposit":  func(r *SettingsRecord) { r.Editable.DepositPercent = -5 },
	} {
		r := good
		mutate(&r)
		if _, err := RestoreSettings(r); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: %v, want ErrInvalidSettings", name, err)
		}
	}
}
