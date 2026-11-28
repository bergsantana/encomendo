package customer

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrPhoneTaken means another customer of the store already has the WhatsApp
// number: 409 phone_taken. It matches shared.ErrConflict. The caller may
// then open the existing customer, found with ByPhone.
var ErrPhoneTaken = fmt.Errorf("customer: whatsapp phone already used: %w", shared.ErrConflict)
