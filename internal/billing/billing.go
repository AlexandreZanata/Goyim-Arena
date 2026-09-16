// Package billing owns the catalog, checkout, subscription and webhooks.
//
// The domain layer (internal/billing/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/billing/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/billing/adapters.
package billing

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "billing"
