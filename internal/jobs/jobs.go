// Package jobs owns persistent execution, leasing and retry.
//
// The domain layer (internal/jobs/domain) holds entities, value objects,
// policies and errors using only the Go standard library. The application
// layer (internal/jobs/application) holds commands, queries, use cases and
// consumer-oriented ports. Adapters arrive in later plan phases and must live
// under internal/jobs/adapters.
package jobs

// ModuleName identifies this module in logs, metrics and audit events.
const ModuleName = "jobs"
