package customer

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ErrNotFound means no customer of the store has the id or phone asked for:
// 404 not_found. Repositories return it; RLS makes another store's customer
// look the same.
var ErrNotFound = fmt.Errorf("customer: not found: %w", shared.ErrNotFound)

// ErrInvalidCustomer means a customer was built from parts that can only be
// wrong through a bug: a zero tenant, customer or address id, or a stored row
// that breaks a rule. It matches no shared
// sentinel, so the API answers 500. What a person can get wrong comes back as
// a shared.ValidationError instead.
var ErrInvalidCustomer = errors.New("customer: invalid customer")

// Field names and pt-BR messages, from the customers section of
// docs/product/design-document.md. The names are those of
// docs/api/openapi.yaml; an address field is written "addresses[2].cep".
// Messages the design document does not list follow its wording.
const (
	fieldName      = "name"
	fieldPhone     = "phone"
	fieldEmail     = "email"
	fieldNotes     = "notes"
	fieldAddresses = "addresses"

	fieldID         = "id"
	fieldLabel      = "label"
	fieldCEP        = "cep"
	fieldStreet     = "street"
	fieldNumber     = "number"
	fieldComplement = "complement"
	fieldDistrict   = "district"
	fieldCity       = "city"
	fieldState      = "state"

	msgNameRequired  = "Informe o nome do cliente."
	msgNameTooLong   = "Use um nome de até 120 caracteres."
	msgNameControl   = "Use um nome sem quebras de linha ou caracteres de controle."
	msgPhoneRequired = "Informe o WhatsApp do cliente."
	msgPhoneInvalid  = "Confira o número: use DDD e 8 ou 9 dígitos."
	msgEmailInvalid  = "Confira o e-mail."
	msgNotesTooLong  = "Use observações de até 2000 caracteres."
	msgNotesInvalid  = "Use observações sem caracteres de controle."
	msgTooManyAddrs  = "Use até 10 endereços."
	msgAddressRepeat = "Este endereço aparece mais de uma vez."

	msgLabelRequired      = "Informe o nome do endereço."
	msgLabelTooLong       = "Use um nome de endereço de até 40 caracteres."
	msgCEPInvalid         = "Confira o CEP: use 8 dígitos."
	msgStreetRequired     = "Informe a rua."
	msgStreetTooLong      = "Use uma rua de até 200 caracteres."
	msgNumberTooLong      = "Use um número de até 20 caracteres."
	msgComplementTooLong  = "Use um complemento de até 100 caracteres."
	msgDistrictTooLong    = "Use um bairro de até 100 caracteres."
	msgCityRequired       = "Informe a cidade."
	msgCityTooLong        = "Use uma cidade de até 100 caracteres."
	msgStateInvalid       = "Escolha a UF."
	msgAddressTextControl = "Use texto sem quebras de linha ou caracteres de controle."
)
