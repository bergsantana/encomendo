// Package fake holds hand-written in-memory fakes of the ports in
// internal/app/shared, for use case tests. Import it as sharedfake (ADR
// 0009). Every fake passes the same contract suite as its real adapter
// (test/contract/sharedports).
//
// The fake TxManager does not roll back other fakes' writes, but it does
// hold events back until the transaction succeeds, so "an event exists only
// if its change committed" holds in use case tests too.
package fake
