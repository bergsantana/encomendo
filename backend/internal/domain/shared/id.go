package shared

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// ErrInvalidID means a string is not a canonical, non-nil UUID. It matches
// ErrInvalid, so a malformed id in a request is a 400.
var ErrInvalidID = fmt.Errorf("invalid id: %w", ErrInvalid)

// ID is a 128-bit identifier written as a canonical UUID. Encomendo generates
// UUIDv7 (time-ordered) ids in Go, never in the database; the typed IDs below
// share this representation so that one context's id cannot be passed where
// another's is expected.
type ID [16]byte

// ParseID reads the canonical 8-4-4-4-12 hex form, in either case. It
// returns ErrInvalidID for any other form and for the nil UUID, which is
// never a real id.
func ParseID(s string) (ID, error) {
	var id ID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return ID{}, ErrInvalidID
	}
	hexDigits := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.Decode(id[:], []byte(hexDigits)); err != nil {
		return ID{}, ErrInvalidID
	}
	if id.IsZero() {
		return ID{}, ErrInvalidID
	}
	return id, nil
}

// NewV7 builds a UUIDv7 from a Unix time in milliseconds and ten random
// bytes. Only the low 48 bits of unixMilli are used (enough until the year
// 10889), and 74 of the 80 random bits survive the version and variant bits.
// It is pure so that both the real generator and the test fakes produce
// valid, time-ordered ids.
func NewV7(unixMilli int64, random [10]byte) ID {
	var id ID
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(unixMilli)) //nolint:gosec // only the low 48 bits are kept, by design of UUIDv7
	copy(id[0:6], ms[2:8])
	id[6] = 0x70 | random[0]&0x0f // version 7
	id[7] = random[1]
	id[8] = 0x80 | random[2]&0x3f // RFC 9562 variant
	copy(id[9:], random[3:])
	return id
}

// String returns the lowercase canonical form.
func (id ID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], id[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], id[10:16])
	return string(b[:])
}

// IsZero reports whether id is the nil UUID, the value of an unset id.
func (id ID) IsZero() bool { return id == ID{} }

// Version returns the UUID version number (7 for generated ids).
func (id ID) Version() int { return int(id[6] >> 4) }

// MarshalText writes the canonical form, or nothing for the unset id, so
// ids stay strings in the JSON of events and cached views (ADR 0009).
func (id ID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText reads what MarshalText writes. It returns ErrInvalidID for
// anything ParseID refuses, other than the empty string.
func (id *ID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

func parseTyped[T ~[16]byte](s string) (T, error) {
	id, err := ParseID(s)
	if err != nil {
		return T{}, err
	}
	return T(id), nil
}

func marshalTyped[T ~[16]byte](id T) ([]byte, error) {
	if ID(id).IsZero() {
		return []byte{}, nil
	}
	return []byte(ID(id).String()), nil
}

func unmarshalTyped[T ~[16]byte](dst *T, b []byte) error {
	if len(b) == 0 {
		*dst = T{}
		return nil
	}
	id, err := parseTyped[T](string(b))
	if err != nil {
		return err
	}
	*dst = id
	return nil
}

// TenantID identifies a tenant (one store).
type TenantID ID

// ParseTenantID parses a tenant id; see ParseID for the errors.
func ParseTenantID(s string) (TenantID, error) { return parseTyped[TenantID](s) }

// String returns the canonical form.
func (id TenantID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id TenantID) IsZero() bool { return id == TenantID{} }

// MarshalText works as ID.MarshalText.
func (id TenantID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *TenantID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// UserID identifies a staff user (owner, admin, staff or viewer of any number
// of tenants).
type UserID ID

// ParseUserID parses a user id; see ParseID for the errors.
func ParseUserID(s string) (UserID, error) { return parseTyped[UserID](s) }

// String returns the canonical form.
func (id UserID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id UserID) IsZero() bool { return id == UserID{} }

// MarshalText works as ID.MarshalText.
func (id UserID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *UserID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// ClientAccountID identifies a global client account (Minhas encomendas).
type ClientAccountID ID

// ParseClientAccountID parses a client account id; see ParseID for the errors.
func ParseClientAccountID(s string) (ClientAccountID, error) {
	return parseTyped[ClientAccountID](s)
}

// String returns the canonical form.
func (id ClientAccountID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id ClientAccountID) IsZero() bool { return id == ClientAccountID{} }

// MarshalText works as ID.MarshalText.
func (id ClientAccountID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *ClientAccountID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// InvitationID identifies an invitation to join a tenant's team.
type InvitationID ID

// ParseInvitationID parses an invitation id; see ParseID for the errors.
func ParseInvitationID(s string) (InvitationID, error) { return parseTyped[InvitationID](s) }

// String returns the canonical form.
func (id InvitationID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id InvitationID) IsZero() bool { return id == InvitationID{} }

// MarshalText works as ID.MarshalText.
func (id InvitationID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *InvitationID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// WorkflowID identifies a status workflow. Together with a version number it
// names one immutable published workflow (ADR 0004).
type WorkflowID ID

// ParseWorkflowID parses a workflow id; see ParseID for the errors.
func ParseWorkflowID(s string) (WorkflowID, error) { return parseTyped[WorkflowID](s) }

// String returns the canonical form.
func (id WorkflowID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id WorkflowID) IsZero() bool { return id == WorkflowID{} }

// MarshalText works as ID.MarshalText.
func (id WorkflowID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *WorkflowID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// SupplierID identifies one of a store's suppliers.
type SupplierID ID

// ParseSupplierID parses a supplier id; see ParseID for the errors.
func ParseSupplierID(s string) (SupplierID, error) { return parseTyped[SupplierID](s) }

// String returns the canonical form.
func (id SupplierID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id SupplierID) IsZero() bool { return id == SupplierID{} }

// MarshalText works as ID.MarshalText.
func (id SupplierID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *SupplierID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// ItemID identifies a catalog item.
type ItemID ID

// ParseItemID parses an item id; see ParseID for the errors.
func ParseItemID(s string) (ItemID, error) { return parseTyped[ItemID](s) }

// String returns the canonical form.
func (id ItemID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id ItemID) IsZero() bool { return id == ItemID{} }

// MarshalText works as ID.MarshalText.
func (id ItemID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *ItemID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// KitID identifies a catalog kit.
type KitID ID

// ParseKitID parses a kit id; see ParseID for the errors.
func ParseKitID(s string) (KitID, error) { return parseTyped[KitID](s) }

// String returns the canonical form.
func (id KitID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id KitID) IsZero() bool { return id == KitID{} }

// MarshalText works as ID.MarshalText.
func (id KitID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *KitID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// CategoryID identifies an item category.
type CategoryID ID

// ParseCategoryID parses a category id; see ParseID for the errors.
func ParseCategoryID(s string) (CategoryID, error) { return parseTyped[CategoryID](s) }

// String returns the canonical form.
func (id CategoryID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id CategoryID) IsZero() bool { return id == CategoryID{} }

// MarshalText works as ID.MarshalText.
func (id CategoryID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *CategoryID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// AttributeID identifies an item field such as "Serve em".
type AttributeID ID

// ParseAttributeID parses an attribute id; see ParseID for the errors.
func ParseAttributeID(s string) (AttributeID, error) { return parseTyped[AttributeID](s) }

// String returns the canonical form.
func (id AttributeID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id AttributeID) IsZero() bool { return id == AttributeID{} }

// MarshalText works as ID.MarshalText.
func (id AttributeID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *AttributeID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// AttributeValueID identifies one value of an item field, such as "CB 500X".
type AttributeValueID ID

// ParseAttributeValueID parses an attribute value id; see ParseID for the errors.
func ParseAttributeValueID(s string) (AttributeValueID, error) {
	return parseTyped[AttributeValueID](s)
}

// String returns the canonical form.
func (id AttributeValueID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id AttributeValueID) IsZero() bool { return id == AttributeValueID{} }

// MarshalText works as ID.MarshalText.
func (id AttributeValueID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *AttributeValueID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// PhotoID identifies a photo of an item or kit.
type PhotoID ID

// ParsePhotoID parses a photo id; see ParseID for the errors.
func ParsePhotoID(s string) (PhotoID, error) { return parseTyped[PhotoID](s) }

// String returns the canonical form.
func (id PhotoID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id PhotoID) IsZero() bool { return id == PhotoID{} }

// MarshalText works as ID.MarshalText.
func (id PhotoID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *PhotoID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// OptionID identifies a customization option of an item.
type OptionID ID

// ParseOptionID parses an option id; see ParseID for the errors.
func ParseOptionID(s string) (OptionID, error) { return parseTyped[OptionID](s) }

// String returns the canonical form.
func (id OptionID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id OptionID) IsZero() bool { return id == OptionID{} }

// MarshalText works as ID.MarshalText.
func (id OptionID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *OptionID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// ChoiceID identifies one choice of a customization option.
type ChoiceID ID

// ParseChoiceID parses a choice id; see ParseID for the errors.
func ParseChoiceID(s string) (ChoiceID, error) { return parseTyped[ChoiceID](s) }

// String returns the canonical form.
func (id ChoiceID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id ChoiceID) IsZero() bool { return id == ChoiceID{} }

// MarshalText works as ID.MarshalText.
func (id ChoiceID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *ChoiceID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// CustomerID identifies a customer of one store.
type CustomerID ID

// ParseCustomerID parses a customer id; see ParseID for the errors.
func ParseCustomerID(s string) (CustomerID, error) { return parseTyped[CustomerID](s) }

// String returns the canonical form.
func (id CustomerID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id CustomerID) IsZero() bool { return id == CustomerID{} }

// MarshalText works as ID.MarshalText.
func (id CustomerID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *CustomerID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// AddressID identifies one address of a customer.
type AddressID ID

// ParseAddressID parses an address id; see ParseID for the errors.
func ParseAddressID(s string) (AddressID, error) { return parseTyped[AddressID](s) }

// String returns the canonical form.
func (id AddressID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id AddressID) IsZero() bool { return id == AddressID{} }

// MarshalText works as ID.MarshalText.
func (id AddressID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *AddressID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// OrderID identifies an order of a store.
type OrderID ID

// ParseOrderID parses an order id; see ParseID for the errors.
func ParseOrderID(s string) (OrderID, error) { return parseTyped[OrderID](s) }

// String returns the canonical form.
func (id OrderID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id OrderID) IsZero() bool { return id == OrderID{} }

// MarshalText works as ID.MarshalText.
func (id OrderID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *OrderID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// OrderLineID identifies one line of an order: a kit, or one loose item.
type OrderLineID ID

// ParseOrderLineID parses an order line id; see ParseID for the errors.
func ParseOrderLineID(s string) (OrderLineID, error) { return parseTyped[OrderLineID](s) }

// String returns the canonical form.
func (id OrderLineID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id OrderLineID) IsZero() bool { return id == OrderLineID{} }

// MarshalText works as ID.MarshalText.
func (id OrderLineID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *OrderLineID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// OrderItemID identifies one item of an order, with its own status and date. A kit line holds one per kit item.
type OrderItemID ID

// ParseOrderItemID parses an order item id; see ParseID for the errors.
func ParseOrderItemID(s string) (OrderItemID, error) { return parseTyped[OrderItemID](s) }

// String returns the canonical form.
func (id OrderItemID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id OrderItemID) IsZero() bool { return id == OrderItemID{} }

// MarshalText works as ID.MarshalText.
func (id OrderItemID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *OrderItemID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }

// PaymentID identifies a payment recorded on an order.
type PaymentID ID

// ParsePaymentID parses a payment id; see ParseID for the errors.
func ParsePaymentID(s string) (PaymentID, error) { return parseTyped[PaymentID](s) }

// String returns the canonical form.
func (id PaymentID) String() string { return ID(id).String() }

// IsZero reports whether the id is unset.
func (id PaymentID) IsZero() bool { return id == PaymentID{} }

// MarshalText works as ID.MarshalText.
func (id PaymentID) MarshalText() ([]byte, error) { return marshalTyped(id) }

// UnmarshalText works as ID.UnmarshalText.
func (id *PaymentID) UnmarshalText(b []byte) error { return unmarshalTyped(id, b) }
