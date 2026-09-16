// Package profiles owns usernames, locale and the public profile.
//
// The domain layer (internal/profiles/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/profiles/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/profiles/adapters.
package profiles

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "profiles"
