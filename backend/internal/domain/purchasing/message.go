package purchasing

import (
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// MessageFacts is what the supplier texts may say. It has no field for the
// client's name, phone, address or order number, so no message can carry
// one (docs/product/design-document.md, System rules).
type MessageFacts struct {
	Store, Supplier string
	Number          Number // zero for "Falar com o fornecedor" before sending
	Items           []Item
}

// OrderMessage returns the text of "Enviar pelo WhatsApp" on a pedido de
// compra, in pt-BR as docs/product/design-document.md ("Supplier messages")
// words it. It sums items with the same catalog item and the same set of
// (option, choice) pairs into one line, keeping the order of first
// appearance.
//
// Product rule: it carries the store's and the supplier's names only, never
// the client's name, phone, address or order number
// (docs/product/design-document.md, System rules). Item holds no such
// field, so no caller can pass one.
func OrderMessage(f MessageFacts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Olá, %s! Aqui é da %s.\nPedido de compra %s:\n", f.Supplier, f.Store, f.Number)
	for _, it := range sumItems(f.Items) {
		fmt.Fprintf(&b, "• %s\n", describe(it))
	}
	b.WriteString("Pode me confirmar a data de entrega?")
	return b.String()
}

// TalkMessage returns the text of "Falar com o fornecedor" about the first
// of f.Items: it asks for the delivery date of a supplier order, or, with
// no Number because nothing was sent yet, for the lead time. It returns ""
// for no item.
func TalkMessage(f MessageFacts) string {
	if len(f.Items) == 0 {
		return ""
	}
	it := describe(f.Items[0])
	if !f.Number.Valid() {
		return fmt.Sprintf("Olá, %s! Aqui é da %s. Sobre %s: consegue me dizer o prazo de entrega?", f.Supplier, f.Store, it)
	}
	return fmt.Sprintf("Olá, %s! Aqui é da %s. Sobre o pedido de compra %s: %s. Consegue me confirmar a data de entrega?",
		f.Supplier, f.Store, f.Number, it)
}

// CancelMessage returns the text of "Avisar o fornecedor": the first of
// f.Items is no longer needed on the supplier order f.Number. It returns ""
// for no item.
func CancelMessage(f MessageFacts) string {
	if len(f.Items) == 0 {
		return ""
	}
	return fmt.Sprintf("Olá, %s! Aqui é da %s. Sobre o pedido de compra %s: não vou mais precisar de %s. Consegue cancelar?",
		f.Supplier, f.Store, f.Number, describe(f.Items[0]))
}

// sumItems adds the quantities of items with the same catalog item and the
// same set of choices. The result keeps the first appearance's name and
// choices, in the order of first appearance.
func sumItems(items []Item) []Item {
	type key struct {
		item    shared.ItemID
		choices string
	}
	at := make(map[key]int, len(items))
	var out []Item
	for _, it := range items {
		k := key{it.CatalogItem, choiceSet(it.Choices)}
		if i, ok := at[k]; ok {
			out[i].Quantity += it.Quantity
			continue
		}
		at[k] = len(out)
		out = append(out, it.clone())
	}
	return out
}

// describe writes "2× Banco de couro (Cor: Preto, Tamanho: G)", with no
// parentheses for an item without choices.
func describe(it Item) string {
	s := fmt.Sprintf("%d× %s", it.Quantity, it.Name)
	if len(it.Choices) == 0 {
		return s
	}
	parts := make([]string, len(it.Choices))
	for i, c := range it.Choices {
		parts[i] = c.Name + ": " + c.Label
	}
	return s + " (" + strings.Join(parts, ", ") + ")"
}
