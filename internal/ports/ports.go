// Package ports holds the small, consumer-oriented interfaces that the
// application layer depends on for the effects its deterministic tests
// cannot tolerate: wall-clock time, cryptographic randomness and
// identifier generation (P02-T02).
//
// The concrete implementations live in internal/platform. There is no
// service locator: constructors and use cases receive these ports as
// ordinary parameters, following docs/ARCHITECTURE.md (Ports and Adapters)
// and AGENTS.md ("abstração sem caso de uso é removida" — only effects with
// more than one relevant execution earn an interface).
package ports

import "time"

// Clock exposes wall-clock time to application and domain code. Production
// wiring uses platform/clockseed.System; deterministic tests use a stub
// defined next to the test, so packages never depend on a global mutable
// clock.
type Clock interface {
	// Now returns the current instant.
	Now() time.Time
}

// Random exposes cryptographically secure random bytes. Production wiring
// uses platform/clockseed.CryptoRandom; tests inject a deterministic reader.
// It is deliberately minimal: callers compose their own token or key formats
// instead of this package guessing one.
type Random interface {
	// Read fills buffer with cryptographically secure random bytes and
	// returns the number of bytes written, or an error when the entropy
	// source fails. It must not return fewer bytes with a nil error.
	Read(buffer []byte) (int, error)
}

// IDGenerator produces opaque, collision-resistant identifiers. Production
// wiring uses platform/clockseed.RandomIDs; tests inject a deterministic
// generator. The identifier format is an implementation detail of the
// concrete type, hidden from consumers.
type IDGenerator interface {
	// NewID returns a fresh, opaque identifier.
	NewID() string
}
