package application

import "errors"

var (
	// ErrPositionNotFound indicates the account holds no position in the
	// Arena. Probing a pair without a position leaks nothing: absence is
	// the same answer for every account.
	ErrPositionNotFound = errors.New("positions: position not found")

	// ErrInitialPositionAlreadySet indicates the account already confirmed
	// a different initial position. The initial position is immutable
	// history: later choices go through a recorded change (P09-T04).
	ErrInitialPositionAlreadySet = errors.New("positions: initial position is already set to a different value")

	// ErrVersionConflict indicates the chain moved since it was read: a
	// concurrent change advanced the projection, so this change rolled
	// back instead of diverging the current position from the chain.
	ErrVersionConflict = errors.New("positions: position version conflict")

	// ErrAccountNotFound indicates the account does not exist.
	ErrAccountNotFound = errors.New("positions: account not found")

	// ErrAccountNotEligible indicates the account may not participate yet,
	// for example because its email is not verified.
	ErrAccountNotEligible = errors.New("positions: account is not eligible to participate")

	// ErrAccountSuspended indicates the account is suspended by moderation.
	ErrAccountSuspended = errors.New("positions: account is suspended")

	// ErrArenaNotFound indicates the Arena does not exist.
	ErrArenaNotFound = errors.New("positions: arena not found")

	// ErrArenaNotOpen indicates the Arena accepts no new positions: only a
	// published Arena is open.
	ErrArenaNotOpen = errors.New("positions: arena is not open to new positions")
)
