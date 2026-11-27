// Package fake holds the hand-written in-memory fake of the customer
// Repository in internal/app/customer, for use case tests: Store emulates
// the tenant_isolation RLS of 0012_customers from the scope in the context
// and its unique number per store. It passes the contract suite its real
// adapter must pass (test/contract/customerports).
package fake
