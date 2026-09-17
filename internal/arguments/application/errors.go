package application

import "errors"

var (
	// ErrArgumentNotFound indicates the argument does not exist (or the
	// parent being resolved does not).
	ErrArgumentNotFound = errors.New("arguments: argument not found")

	// ErrParentNotFound indicates the parent argument of a reply does not
	// exist.
	ErrParentNotFound = errors.New("arguments: parent argument not found")

	// ErrParentNotAvailable indicates the parent exists but cannot receive
	// replies: it belongs to another Arena or is no longer published.
	ErrParentNotAvailable = errors.New("arguments: parent argument is not available for replies")

	// ErrReplyDepthExceeded indicates the reply would exceed the depth
	// policy of the domain (REQ-ARG-02: one recursion level in the MVP).
	ErrReplyDepthExceeded = errors.New("arguments: reply depth exceeds the accepted limit")

	// ErrDuplicateIdempotencyKey is the internal signal of a concurrent
	// attempt with the same key: the use case resolves it into a replay.
	ErrDuplicateIdempotencyKey = errors.New("arguments: idempotency key already used")

	// ErrArgumentNotWithdrawable indicates the argument cannot be withdrawn
	// in its current state: a moderation removal is never overridden by the
	// author.
	ErrArgumentNotWithdrawable = errors.New("arguments: argument cannot be withdrawn in its current state")

	// ErrInsufficientInk indicates the author cannot pay the publication
	// cost: the wallet debit was refused and nothing was written.
	ErrInsufficientInk = errors.New("arguments: insufficient ink for the publication cost")

	// ErrAccountNotFound indicates the author account does not exist.
	ErrAccountNotFound = errors.New("arguments: account not found")

	// ErrAccountNotEligible indicates the account may not publish yet, for
	// example because its email is not verified.
	ErrAccountNotEligible = errors.New("arguments: account is not eligible to publish")

	// ErrAccountSuspended indicates the account is suspended by moderation.
	ErrAccountSuspended = errors.New("arguments: account is suspended")

	// ErrArenaNotFound indicates the Arena does not exist.
	ErrArenaNotFound = errors.New("arguments: arena not found")

	// ErrArenaNotOpen indicates the Arena accepts no new arguments: only a
	// published Arena is open.
	ErrArenaNotOpen = errors.New("arguments: arena is not open to new arguments")

	// ErrInvalidCursor indicates a malformed, forged or version-mismatched
	// keyset cursor. Unknown values are never reflected back to callers.
	ErrInvalidCursor = errors.New("arguments: argument list cursor is invalid")

	// ErrWeakCursorSecret indicates the configured cursor signing secret is
	// shorter than the 256-bit minimum.
	ErrWeakCursorSecret = errors.New("arguments: argument cursor secret must be at least 32 bytes")
)
