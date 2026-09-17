// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the billing module.
package application

import "errors"

var (
	// ErrArenaAlreadyConsumed indicates the arena already consumed a pass that
	// belongs to another account: a pass can never be transferred between
	// accounts.
	ErrArenaAlreadyConsumed = errors.New("application: arena pass was already consumed by another account")

	// ErrInvalidCursor indicates a malformed, forged or version-mismatched
	// history cursor. Unknown values are never reflected back to callers.
	ErrInvalidCursor = errors.New("application: pass history cursor is invalid")

	// ErrWeakHistoryCursorSecret indicates the configured cursor signing
	// secret is shorter than the 256-bit minimum.
	ErrWeakHistoryCursorSecret = errors.New("application: pass history cursor secret must be at least 32 bytes")
)
