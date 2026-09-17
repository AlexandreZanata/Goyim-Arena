// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the wallet module.
package application

import "errors"

var (
	// ErrIdempotencyMismatch indicates the idempotency key was already used
	// by a different account. Replaying another account's operation is
	// refused instead of leaking its reference.
	ErrIdempotencyMismatch = errors.New("application: idempotency key was already used by another account")
)
