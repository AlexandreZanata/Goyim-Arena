// Package persuasion owns eligibility, attributions and public metrics.
//
// The domain layer (internal/persuasion/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/persuasion/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/persuasion/adapters.
package persuasion

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "persuasion"
