// Package shared holds the driven ports every use case needs: TxManager,
// Cache, Clock, IDGenerator and EventPublisher, plus the Scope that tells the
// TxManager whose rows a transaction may see.
//
// Import it as appshared, because the domain's shared package is imported
// next to it in every use case (ADR 0009; golangci-lint enforces the alias).
// Fakes of every port live in the fake subpackage, imported as sharedfake,
// and the port contract suites in test/contract/sharedports run against
// both the fakes and the real adapters.
package shared
