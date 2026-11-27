package workflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// valid returns a fresh valid status list to break one rule at a time.
func valid() []Status {
	return []Status{
		st("draft", "Rascunho", "Pedido recebido", PhaseDraft),
		st("awaiting_deposit", "Aguardando sinal", "Aguardando o sinal", PhaseAwaitingDeposit),
		st("purchasing", "Compras", "Pedido confirmado", PhaseSourcing),
		st("ready", "Pronto", "Pronto", PhaseReady),
		st("delivered", "Entregue", "Entregue", PhaseHandedOver),
	}
}

// wantRule fails unless err is ErrInvalidWorkflow caused by rule.
func wantRule(t *testing.T, err, rule error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidWorkflow) || !errors.Is(err, shared.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalidWorkflow matching ErrInvalid", err)
	}
	if !errors.Is(err, rule) {
		t.Fatalf("err = %v, want rule %q", err, rule)
	}
}

func TestValidate_Valid(t *testing.T) {
	t.Parallel()
	if err := Validate(valid()); err != nil {
		t.Fatalf("Validate = %v", err)
	}
}

func TestValidate_Empty(t *testing.T) {
	t.Parallel()
	wantRule(t, Validate(nil), errEmpty)
	wantRule(t, Validate([]Status{}), errEmpty)
}

func TestValidate_DuplicateKey(t *testing.T) {
	t.Parallel()
	s := valid()
	s[3].Key = "purchasing"
	wantRule(t, Validate(s), errDuplicateKey)
}

func TestValidate_ReservedKey(t *testing.T) {
	t.Parallel()
	s := valid()
	s[2].Key = "cancelled"
	wantRule(t, Validate(s), errReservedKey)
}

func TestValidate_KeyFormat(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"", "a", "Draft", "1abc", "_abc", "has space", "com-hifen", "açúcar",
		strings.Repeat("a", 64),
	} {
		s := valid()
		s[2].Key = StatusKey(key)
		wantRule(t, Validate(s), errKeyFormat)
	}
	for _, key := range []string{"ab", "a1", "a_b", strings.Repeat("a", 63)} {
		s := valid()
		s[2].Key = StatusKey(key)
		if err := Validate(s); err != nil {
			t.Errorf("key %q: %v", key, err)
		}
	}
}

func TestValidate_LabelLength(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"", strings.Repeat("a", 61)} {
		s := valid()
		s[2].Label = label
		wantRule(t, Validate(s), errLabelLength)
	}
	// Characters, not bytes: 60 accented letters are 120 bytes.
	for _, label := range []string{"a", strings.Repeat("ç", 60)} {
		s := valid()
		s[2].Label = label
		if err := Validate(s); err != nil {
			t.Errorf("label of %d runes: %v", len([]rune(label)), err)
		}
	}
}

func TestValidate_ClientLabelLength(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"", strings.Repeat("a", 61)} {
		s := valid()
		s[2].ClientLabel = label
		wantRule(t, Validate(s), errClientLabelLength)
	}
}

func TestValidate_CancelledPhase(t *testing.T) {
	t.Parallel()
	s := valid()
	s[2].Phase = PhaseCancelled
	wantRule(t, Validate(s), errCancelledPhase)
}

func TestValidate_UnknownPhase(t *testing.T) {
	t.Parallel()
	s := valid()
	s[2].Phase = "shipped"
	wantRule(t, Validate(s), errUnknownPhase)
	s[2].Phase = ""
	wantRule(t, Validate(s), errUnknownPhase)
}

func TestValidate_PhaseGoesBack(t *testing.T) {
	t.Parallel()
	s := valid()
	// ready before sourcing: the phase of purchasing is lower than ready.
	s[2], s[3] = s[3], s[2]
	wantRule(t, Validate(s), errPhaseGoesBack)
}

func TestValidate_MissingPhase(t *testing.T) {
	t.Parallel()
	for _, missing := range []Phase{PhaseDraft, PhaseAwaitingDeposit, PhaseReady, PhaseHandedOver} {
		var s []Status
		for _, x := range valid() {
			if x.Phase != missing {
				s = append(s, x)
			}
		}
		wantRule(t, Validate(s), errMissingPhase)
	}
}

func TestValidate_FirstNotDraft(t *testing.T) {
	t.Parallel()
	s := valid()
	s[0], s[1] = s[1], s[0]
	wantRule(t, Validate(s), errFirstNotDraft)
}

func TestValidate_LastNotHandedOver(t *testing.T) {
	t.Parallel()
	s := valid()
	s[3], s[4] = s[4], s[3]
	wantRule(t, Validate(s), errLastNotHandedOver)
}

func TestValidate_AllowsRepeatedPhases(t *testing.T) {
	t.Parallel()
	s := valid()
	s = append(s[:3], append([]Status{
		st("items_arriving", "Itens chegando", "Itens a caminho", PhaseSourcing),
		st("assembling", "Em montagem", "Em montagem", PhaseAssembling),
		st("testing", "Em teste", "Em teste", PhaseAssembling),
	}, s[3:]...)...)
	if err := Validate(s); err != nil {
		t.Fatalf("Validate = %v", err)
	}
}
