// Package positions owns the initial position, the current position and
// position changes over time.
//
// The domain layer (internal/positions/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/positions/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/positions/adapters.
package positions

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "positions"
