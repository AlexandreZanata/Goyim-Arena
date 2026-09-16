// Package audit owns administrative events and their integrity guarantees.
//
// The domain layer (internal/audit/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/audit/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/audit/adapters.
package audit

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "audit"
