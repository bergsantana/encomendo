package purchasing_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestOrderMessage_SumsSameItemAndChoices(t *testing.T) {
	t.Parallel()
	black, big, small := choice("Cor", "Preto"), choice("Tamanho", "G"), choice("Tamanho", "P")
	seat := item("Banco de couro CB 500X", 1, black)
	pedal := item("Pedal", 1)
	// The same catalog item and choices from other orders, the choices in
	// another order, and a different choice set.
	seat2 := seat
	seat2.Order, seat2.OrderItem, seat2.Quantity = shared.OrderID(testkit.NextID()), shared.OrderItemID(testkit.NextID()), 1
	pedal2 := pedal
	pedal2.Order, pedal2.OrderItem, pedal2.Quantity = shared.OrderID(testkit.NextID()), shared.OrderItemID(testkit.NextID()), 3
	grip := item("Manopla", 1, black, big)
	grip2 := grip
	grip2.Choices = []purchasing.Choice{big, black}
	grip3 := grip
	grip3.Choices = []purchasing.Choice{black, small}

	got := purchasing.OrderMessage(purchasing.MessageFacts{
		Store: "Moto Custom", Supplier: "Moto Peças", Number: 12,
		Items: []purchasing.Item{seat, pedal, seat2, grip, pedal2, grip2, grip3},
	})
	want := "Olá, Moto Peças! Aqui é da Moto Custom.\n" +
		"Pedido de compra C-0012:\n" +
		"• 2× Banco de couro CB 500X (Cor: Preto)\n" +
		"• 4× Pedal\n" +
		"• 2× Manopla (Cor: Preto, Tamanho: G)\n" +
		"• 1× Manopla (Cor: Preto, Tamanho: P)\n" +
		"Pode me confirmar a data de entrega?"
	if got != want {
		t.Errorf("OrderMessage =\n%s\nwant\n%s", got, want)
	}
}

func TestOrderMessage_DifferentCatalogItemsStaySeparate(t *testing.T) {
	t.Parallel()
	a, b := item("Pedal", 1), item("Pedal", 1) // same name, different catalog items
	got := purchasing.OrderMessage(purchasing.MessageFacts{Store: "L", Supplier: "F", Number: 1, Items: []purchasing.Item{a, b}})
	if strings.Count(got, "• 1× Pedal") != 2 {
		t.Errorf("message = %q, want two lines", got)
	}
}

func TestTalkAndCancelMessages(t *testing.T) {
	t.Parallel()
	it := item("Banco de couro", 2, choice("Cor", "Preto"))
	f := purchasing.MessageFacts{Store: "Moto Custom", Supplier: "Moto Peças", Number: 12, Items: []purchasing.Item{it}}

	if got, want := purchasing.TalkMessage(f), "Olá, Moto Peças! Aqui é da Moto Custom. Sobre o pedido de compra C-0012: 2× Banco de couro (Cor: Preto). Consegue me confirmar a data de entrega?"; got != want {
		t.Errorf("TalkMessage = %q, want %q", got, want)
	}
	f.Number = 0
	if got, want := purchasing.TalkMessage(f), "Olá, Moto Peças! Aqui é da Moto Custom. Sobre 2× Banco de couro (Cor: Preto): consegue me dizer o prazo de entrega?"; got != want {
		t.Errorf("TalkMessage before sending = %q, want %q", got, want)
	}
	f.Number = 12
	if got, want := purchasing.CancelMessage(f), "Olá, Moto Peças! Aqui é da Moto Custom. Sobre o pedido de compra C-0012: não vou mais precisar de 2× Banco de couro (Cor: Preto). Consegue cancelar?"; got != want {
		t.Errorf("CancelMessage = %q, want %q", got, want)
	}
	f.Items = nil
	if purchasing.TalkMessage(f) != "" || purchasing.CancelMessage(f) != "" {
		t.Error("a message about no item must be empty")
	}
}

// clientData is what a client of an order has. The purchasing package has
// no field for any of it; the test builds it anyway to prove no text holds it.
type clientData struct {
	name, phone, address, orderNumber string
}

func TestOrderMessage_NeverCarriesClientData(t *testing.T) {
	t.Parallel()
	clients := []clientData{
		{"Maria Aparecida Souza", "(92) 99999-1234", "Rua das Flores, 120, Manaus", "P-0231"},
		{"João Pedro Lima", "5592988887777", "Av. Djalma Batista 900", "P-0232"},
	}
	// Each client has an order with a bought item. The item a line copies
	// from the order has no field for the client, so nothing of the client
	// can reach the text.
	var items []purchasing.Item
	for i := range clients {
		items = append(items, purchasing.Item{
			Order: shared.OrderID(testkit.NextID()), OrderItem: shared.OrderItemID(testkit.NextID()),
			CatalogItem: shared.ItemID(testkit.NextID()), Name: "Banco de couro", Quantity: i + 1,
			Choices: []purchasing.Choice{choice("Cor", "Preto")},
		})
	}
	facts := purchasing.MessageFacts{Store: "Moto Custom", Supplier: "Moto Peças", Number: 12, Items: items}
	texts := map[string]string{
		"OrderMessage":  purchasing.OrderMessage(facts),
		"TalkMessage":   purchasing.TalkMessage(facts),
		"CancelMessage": purchasing.CancelMessage(facts),
	}
	facts.Number = 0
	texts["TalkMessage unsent"] = purchasing.TalkMessage(facts)
	for name, text := range texts {
		for _, c := range clients {
			for _, secret := range []string{c.name, c.phone, c.address, c.orderNumber, "P-0", strings.Fields(c.name)[0]} {
				if strings.Contains(text, secret) {
					t.Errorf("%s carries %q:\n%s", name, secret, text)
				}
			}
		}
	}
}

var longDigits = regexp.MustCompile(`\d{8,}`)

func FuzzOrderMessage_Names(f *testing.F) {
	f.Add("Moto Custom", "Moto Peças", "Banco", "Cor", "Preto")
	f.Add("L\u00e9a 5592999991234", "F\n", "Pedal (G)", "", "")
	f.Add("P-0231", "Maria", "💥", "Tamanho", "G, M")
	f.Fuzz(func(t *testing.T, store, supplier, name, option, label string) {
		it := purchasing.Item{
			Order: shared.OrderID(testkit.NextID()), OrderItem: shared.OrderItemID(testkit.NextID()),
			CatalogItem: shared.ItemID(testkit.NextID()), Name: name, Quantity: 2,
			Choices: []purchasing.Choice{{Name: option, Label: label}},
		}
		facts := purchasing.MessageFacts{Store: store, Supplier: supplier, Number: 12, Items: []purchasing.Item{it, it}}
		inputs := strings.Join([]string{store, supplier, name, option, label}, "\x00")
		for _, text := range []string{purchasing.OrderMessage(facts), purchasing.TalkMessage(facts), purchasing.CancelMessage(facts)} {
			// "P-0" is how an order number starts: it may be there only
			// when a name put it there.
			if strings.Contains(text, "P-0") && !strings.Contains(inputs, "P-0") {
				t.Fatalf("message made a P-0 out of nothing: %q", text)
			}
			// Nor may a phone-looking run of digits appear that no name had.
			for _, run := range longDigits.FindAllString(text, -1) {
				if !strings.Contains(inputs, run) {
					t.Fatalf("message made the digits %q out of nothing: %q", run, text)
				}
			}
		}
	})
}
