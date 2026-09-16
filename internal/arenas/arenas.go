// Package arenas owns drafts, publication, closure, category and language.
//
// The domain layer (internal/arenas/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/arenas/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/arenas/adapters.
package arenas

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "arenas"
