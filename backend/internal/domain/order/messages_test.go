package order_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func facts(t *testing.T) order.MessageFacts {
	t.Helper()
	return order.MessageFacts{
		Store: "Alta RPM", CustomerFirstName: "João", Number: "P-0231",
		Total: brl(t, 123_456), Deposit: brl(t, 61_728), Refund: brl(t, 0),
		OldDate: date(t, "2026-12-21"), NewDate: date(t, "2027-01-05"),
	}
}

// The texts of docs/product/design-document.md, WhatsApp messages.
func TestMessages_MatchTheDesignDocument(t *testing.T) {
	t.Parallel()

	f := facts(t)
	link := "https://altarpm.encomendo.com.br/p/abc"
	tests := map[string]struct{ got, want string }{
		"tracking": {order.TrackingMessage(f, link),
			"Olá, João! Aqui é da Alta RPM. Acompanhe seu pedido P-0231 por este link: https://altarpm.encomendo.com.br/p/abc"},
		"created": {order.CreatedMessage(f, link),
			"Olá, João! Aqui é da Alta RPM. Recebemos seu pedido P-0231, total R$ 1.234,56. O sinal é R$ 617,28. Acompanhe por este link: https://altarpm.encomendo.com.br/p/abc"},
		"date": {order.DateChangedMessage(f),
			"Olá, João! Aqui é da Alta RPM. A entrega do seu pedido P-0231 mudou de 21/12/2026 para 05/01/2027. Qualquer dúvida, é só responder esta mensagem."},
		"changed": {order.ChangedMessage(f),
			"Olá, João! Aqui é da Alta RPM. Como combinamos, seu pedido P-0231 foi alterado. Novo total: R$ 1.234,56. Entrega prevista: 05/01/2027. Qualquer dúvida, é só responder esta mensagem."},
	}
	for name, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", name, tt.got, tt.want)
		}
	}
	f.Reason, f.Refund = "Atraso do fornecedor", brl(t, 5_000_000)
	if got := order.DateChangedMessage(f); !strings.Contains(got, "para 05/01/2027. Motivo: Atraso do fornecedor. Qualquer") {
		t.Errorf("date with reason = %q", got)
	}
	if got := order.ChangedMessage(f); !strings.Contains(got, "Entrega prevista: 05/01/2027. Valor a devolver: R$ 50.000,00. Qualquer") {
		t.Errorf("changed with refund = %q", got)
	}
}

func FuzzMessages_NeverNameSupplierOrRisk(f *testing.F) {
	f.Add("Alta RPM", "João", "P-0001", int64(100), "")
	f.Add("Loja", "Ana", "P-99999", int64(123456789), "Chuva")
	f.Fuzz(func(t *testing.T, store, name, number string, cents int64, reason string) {
		for _, s := range []string{store, name, number, reason} {
			if strings.Contains(s, "/p/") || strings.Contains(strings.ToLower(s), "pode atrasar") || strings.Contains(strings.ToLower(s), "fornecedor") {
				t.Skip()
			}
		}
		m, err := shared.NewMoney(cents, "BRL")
		if err != nil {
			t.Skip()
		}
		ff := order.MessageFacts{Store: store, CustomerFirstName: name, Number: number, Total: m, Deposit: m, Refund: m, Reason: reason,
			OldDate: testkit.Date(t, "2026-12-01"), NewDate: testkit.Date(t, "2026-12-09")}
		for _, msg := range []string{order.DateChangedMessage(ff), order.ChangedMessage(ff)} {
			low := strings.ToLower(msg)
			// Only the create and tracking-link messages take a link (ADR
			// 0030, Amendments); none may say "Pode atrasar" or name a
			// supplier (design document, System rules).
			if strings.Contains(msg, "/p/") || strings.Contains(low, "pode atrasar") || strings.Contains(low, "fornecedor") {
				t.Errorf("message leaks: %q", msg)
			}
		}
	})
}

// TestMessageFacts_HasNoInternalField fails when someone adds a field that
// could carry a supplier, a cost or an alert into a client message.
func TestMessageFacts_HasNoInternalField(t *testing.T) {
	t.Parallel()

	refused := []string{"supplier", "cost", "margin", "alert", "risk", "late", "phone", "link", "url"}
	ft := reflect.TypeOf(order.MessageFacts{})
	for i := range ft.NumField() {
		name := strings.ToLower(ft.Field(i).Name)
		for _, r := range refused {
			if strings.Contains(name, r) {
				t.Errorf("MessageFacts.%s looks like internal data (%s)", ft.Field(i).Name, r)
			}
		}
	}
}

func TestEvents_CarryNoPersonalData(t *testing.T) {
	t.Parallel()

	events := []any{
		order.Created{}, order.Advanced{}, order.DepositConfirmed{}, order.Released{}, order.PaymentRecorded{},
		order.PaymentVoided{}, order.ItemMoved{}, order.Changed{}, order.DateChanged{}, order.Cancelled{},
	}
	refused := []string{"name", "phone", "email", "address", "customer", "token", "tracking", "amount", "cost", "supplier"}
	names := map[string]bool{}
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(b, &fields); err != nil {
			t.Fatal(err)
		}
		for k := range fields {
			for _, r := range refused {
				if strings.Contains(strings.ToLower(k), r) {
					t.Errorf("%T has field %q", e, k)
				}
			}
		}
		n := e.(interface{ EventName() string }).EventName()
		if !strings.HasPrefix(n, "order.") || names[n] {
			t.Errorf("%T name %q is not a unique order.* name", e, n)
		}
		names[n] = true
	}
}
