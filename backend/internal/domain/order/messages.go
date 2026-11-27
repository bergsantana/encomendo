package order

import (
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// MessageFacts is what the WhatsApp texts may say. It has no field for a
// supplier, a cost or an alert, so no message can carry one
// (docs/product/design-document.md, System rules and WhatsApp messages).
type MessageFacts struct {
	Store, CustomerFirstName, Number string
	Total, Deposit, Refund           shared.Money
	OldDate, NewDate                 shared.CivilDate
	Reason                           string
}

// TrackingMessage is the text that sends the tracking link.
func TrackingMessage(f MessageFacts, link string) string {
	return fmt.Sprintf("Olá, %s! Aqui é da %s. Acompanhe seu pedido %s por este link: %s",
		f.CustomerFirstName, f.Store, f.Number, link)
}

// CreatedMessage is the text sent with a new order ("Salvar e enviar ao
// cliente"), with its tracking link.
func CreatedMessage(f MessageFacts, link string) string {
	return fmt.Sprintf("Olá, %s! Aqui é da %s. Recebemos seu pedido %s, total %s. O sinal é %s. Acompanhe por este link: %s",
		f.CustomerFirstName, f.Store, f.Number, formatMoney(f.Total), formatMoney(f.Deposit), link)
}

// DateChangedMessage warns the client that the delivery date changed. It
// takes no link: only the create and tracking-link answers carry one (ADR
// 0030, Amendments).
func DateChangedMessage(f MessageFacts) string {
	reason := ""
	if f.Reason != "" {
		reason = " Motivo: " + strings.TrimSuffix(f.Reason, ".") + "."
	}
	return fmt.Sprintf("Olá, %s! Aqui é da %s. A entrega do seu pedido %s mudou de %s para %s.%s Qualquer dúvida, é só responder esta mensagem.",
		f.CustomerFirstName, f.Store, f.Number, formatDate(f.OldDate), formatDate(f.NewDate), reason)
}

// ChangedMessage sums up a change made after a payment, as agreed with the
// client. It takes no link.
func ChangedMessage(f MessageFacts) string {
	refund := ""
	if f.Refund.Cents() > 0 {
		refund = " Valor a devolver: " + formatMoney(f.Refund) + "."
	}
	return fmt.Sprintf("Olá, %s! Aqui é da %s. Como combinamos, seu pedido %s foi alterado. Novo total: %s. Entrega prevista: %s.%s Qualquer dúvida, é só responder esta mensagem.",
		f.CustomerFirstName, f.Store, f.Number, formatMoney(f.Total), formatDate(f.NewDate), refund)
}

// formatMoney writes an amount the Brazilian way: "R$ 1.234,56". Other
// currencies keep their code in front.
func formatMoney(m shared.Money) string {
	cents := m.Cents()
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	whole := fmt.Sprintf("%d", cents/100)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	symbol := m.Currency()
	if symbol == "BRL" {
		symbol = "R$"
	}
	return fmt.Sprintf("%s%s %s,%02d", sign, symbol, b.String(), cents%100)
}

// formatDate writes a day as "dd/mm/aaaa".
func formatDate(d shared.CivilDate) string {
	s := d.String()
	if len(s) != len("2006-01-02") {
		return ""
	}
	return s[8:10] + "/" + s[5:7] + "/" + s[0:4]
}
