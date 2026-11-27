package supplier

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrNotFound means no supplier of the store has the id asked for: 404
// not_found. Repositories return it; RLS makes another store's supplier look
// the same.
var ErrNotFound = fmt.Errorf("supplier: not found: %w", shared.ErrNotFound)

// Errors for records built from parts that can only be wrong through a bug:
// a zero id, an unknown role, or a stored row that breaks a rule. They match
// no shared sentinel, so the API answers 500. What a person can get wrong
// (name, phone, lead days, cost) comes back as a shared.ValidationError
// instead.
var (
	ErrInvalidSupplier     = errors.New("supplier: invalid supplier")
	ErrInvalidSupplierItem = errors.New("supplier: invalid supplier item")
)

// Field names and pt-BR messages, from the "Catálogo" section of
// docs/product/design-document.md. The names are those of docs/api/openapi.yaml.
const (
	fieldName          = "name"
	fieldWhatsAppPhone = "whatsappPhone"
	fieldLeadDays      = "leadDays"
	fieldCostCents     = "costCents"

	msgNameRequired  = "Informe o nome do fornecedor."
	msgNameTooLong   = "Use um nome de até 120 caracteres."
	msgNameControl   = "Use um nome sem quebras de linha ou caracteres de controle."
	msgPhoneRequired = "Informe o WhatsApp do fornecedor."
	msgPhoneInvalid  = "Confira o número: use DDD e 8 ou 9 dígitos."
	msgLeadDays      = "Use um prazo de 0 a 365 dias."
	msgCost          = "Informe um custo de zero ou mais."
)
