// Package transparency owns public aggregates and versioned exports.
//
// The domain layer (internal/transparency/domain) holds entities, value
// objects, policies and errors using only the Go standard library. The
// application layer (internal/transparency/application) holds commands,
// queries, use cases and consumer-oriented ports. Adapters arrive in later
// plan phases and must live under internal/transparency/adapters.
package transparency

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "transparency"
