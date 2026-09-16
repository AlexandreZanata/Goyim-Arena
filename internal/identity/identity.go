// Package identity owns accounts, credentials, sessions and recovery.
//
// The domain layer (internal/identity/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/identity/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/identity/adapters.
package identity

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "identity"
