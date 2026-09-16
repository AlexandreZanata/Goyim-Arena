// Package moderation owns reports, decisions, appeals and sanctions.
//
// The domain layer (internal/moderation/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/moderation/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/moderation/adapters.
package moderation

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "moderation"
