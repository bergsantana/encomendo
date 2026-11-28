package notify

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// maxMessageRunes bounds a prefilled WhatsApp text. The texts of
// docs/product/design-document.md are a few hundred characters; the bound
// keeps a free-text reason from making a link phones refuse to open.
const maxMessageRunes = 2000

// WhatsAppLinks builds wa.me links with a prefilled text: the user taps one
// and sends the message from their own WhatsApp. Nothing is sent by the
// server, and no paid WhatsApp API is used (docs/architecture.md section 2,
// rule 9).
type WhatsAppLinks struct{}

// NewWhatsAppLinks returns the link builder.
func NewWhatsAppLinks() WhatsAppLinks { return WhatsAppLinks{} }

// Link returns https://wa.me/{phone}?text={text}, the text percent-encoded
// (accents, line breaks, "&" and "+" included; spaces as %20). It returns shared.ErrInvalid
// unless phone is 55 + DDD + 8 or 9 digits, digits only, and the text has
// 1 to 2000 characters of valid UTF-8.
func (WhatsAppLinks) Link(phone, text string) (string, error) {
	if !shared.ValidWhatsAppPhone(phone) {
		// The number is left out of the error: a customer's phone is never
		// written to an error that could reach a log (backend/CLAUDE.md).
		return "", fmt.Errorf("wa.me link: invalid phone: %w", shared.ErrInvalid)
	}
	n := utf8.RuneCountInString(text)
	if n == 0 || n > maxMessageRunes || !utf8.ValidString(text) {
		return "", fmt.Errorf("wa.me link: text of %d characters: %w", n, shared.ErrInvalid)
	}
	// QueryEscape writes a space as "+", which some WhatsApp clients show
	// as a literal plus; "%20" is read the same everywhere. A "+" in the
	// text is already "%2B", so the swap cannot change it.
	return "https://wa.me/" + phone + "?text=" + strings.ReplaceAll(url.QueryEscape(text), "+", "%20"), nil
}
