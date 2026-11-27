// Package customer owns the store's customers and their addresses: the person
// an order is for, as one store knows them.
//
// Rules it enforces:
//   - A customer belongs to one store. The same person at two stores is two
//     customers, and nothing here links them (docs/product/design-document.md,
//     customers section).
//   - A customer has a name of 1 to 120 characters with no control
//     characters, a WhatsApp number that wa.me links accept
//     (shared.ValidWhatsAppPhone), an optional e-mail, and notes of up to
//     2000 characters. Whether the phone is unique in the store is a
//     database rule; the use case maps it to phone_taken.
//   - A customer has up to 10 addresses. Each has a label of 1 to 40
//     characters, a CEP of 8 digits, a street and a city, and a state that is
//     one of the 27 UF codes.
//   - A customer changes only with the version it was read at: Replace takes
//     the version the client sent, so two people editing the same customer
//     never overwrite each other.
//   - A customer may be linked to a client account (Minhas encomendas). The
//     link is set only from M7; until then it is always unset.
//
// Customers are internal data: the store page, tracking and the client area
// never carry a customer's other addresses, notes or history.
//
// It imports only the standard library and other domain packages.
package customer
