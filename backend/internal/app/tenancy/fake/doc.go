// Package fake holds hand-written in-memory fakes of the ports in
// internal/app/tenancy, for use case tests: Store (stores, settings, teams
// and invitations, with RLS emulated from the scope in the context), Users,
// Workflows and Mailer. Every fake passes the contract suite its real
// adapter must pass (test/contract/tenancyports).
package fake
