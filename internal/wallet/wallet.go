// Package wallet owns the append-only INK ledger, buckets and atomic
// consumption.
//
// The domain layer (internal/wallet/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/wallet/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/wallet/adapters.
package wallet

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "wallet"
