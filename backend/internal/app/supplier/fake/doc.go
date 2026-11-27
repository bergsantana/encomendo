// Package fake holds the hand-written in-memory fake of the supplier
// Repository in internal/app/supplier, for use case tests: Store emulates
// the staff-only RLS of 0009_suppliers from the scope in the context. It
// passes the contract suite its real adapter must pass
// (test/contract/supplierports).
package fake
