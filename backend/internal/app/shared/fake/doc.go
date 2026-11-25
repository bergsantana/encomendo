// Package fake holds hand-written in-memory fakes of the ports in
// internal/app/shared, for use case tests. Import it as sharedfake (ADR
// 0009). Every fake passes the same contract suite as its real adapter
// (test/contract/sharedports).
//
// The fake TxManager holds events back until the transaction succeeds, and
// undoes the writes repository fakes register with OnRollback when it
// fails, so "a change and its event exist only if the transaction
// committed" holds in use case tests too.
package fake
