// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the wallet module.
package application

import "errors"

var (
	// ErrIdempotencyMismatch indicates the idempotency key was already used
	// by another account. Replaying another account's operation is
	// refused instead of leaking its reference.
	ErrIdempotencyMismatch = errors.New("application: idempotency key was already used by another account")

	// ErrInvalidCursor indicates a malformed, foreign or version-mismatched
	// statement cursor. Unknown values are never reflected back to callers.
	ErrInvalidCursor = errors.New("application: statement cursor is invalid")

	// ErrWalletNotFound indicates the account has no wallet yet, so no free
	// cycle has ever started for it.
	ErrWalletNotFound = errors.New("application: wallet not found")
)
