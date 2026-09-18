// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the profiles module.
package application

import "errors"

var (
	// ErrProfileNotFound indicates that no profile exists for the account.
	ErrProfileNotFound = errors.New("application: profile not found")

	// ErrProfileAlreadyExists indicates that the account already owns a profile.
	ErrProfileAlreadyExists = errors.New("application: profile already exists for this account")

	// ErrUsernameTaken indicates that another account already holds the
	// requested username (normalized comparison).
	ErrUsernameTaken = errors.New("application: username is already taken")

	// ErrAccountNotEligible indicates that the account cannot own or mutate a
	// profile because it is missing, unverified, suspended or deleted.
	ErrAccountNotEligible = errors.New("application: account is not eligible to own or mutate a profile")

	// ErrInvalidExportConfig indicates the personal export use cases could
	// not be built from the given dependencies.
	ErrInvalidExportConfig = errors.New("application: personal export configuration is invalid")

	// ErrExportNotFound indicates the export is unknown, belongs to another
	// account or is not publicly addressable; foreign and missing records
	// are deliberately indistinguishable.
	ErrExportNotFound = errors.New("application: personal export not found")

	// ErrExportUnavailable indicates the export is not ready, has expired or
	// exhausted its download budget.
	ErrExportUnavailable = errors.New("application: personal export is not available")

	// ErrInvalidExportToken indicates the presented download capability does
	// not match the record; the token is never echoed back.
	ErrInvalidExportToken = errors.New("application: personal export token is invalid")

	// ErrUnknownSession indicates the session carrying the request is not
	// resolvable; it denies like a stale session instead of being treated
	// as fresh.
	ErrUnknownSession = errors.New("application: session is unknown")

	// ErrInvalidDeletionConfig indicates the deletion use cases could not be
	// built from the given dependencies.
	ErrInvalidDeletionConfig = errors.New("application: account deletion configuration is invalid")

	// ErrDeletionRequestNotFound indicates the account has no deletion
	// request.
	ErrDeletionRequestNotFound = errors.New("application: account deletion request not found")

	// ErrDeletionNotCancellable indicates the request is terminal or its
	// cooldown elapsed: the holder can no longer cancel it.
	ErrDeletionNotCancellable = errors.New("application: account deletion request is not cancellable")

	// ErrDeletionNotExecutable indicates the request is missing, terminal or
	// still inside its cooldown window.
	ErrDeletionNotExecutable = errors.New("application: account deletion request is not executable")

	// ErrInvalidCancelReason indicates the cancellation reason exceeds the
	// accepted bound.
	ErrInvalidCancelReason = errors.New("application: account deletion cancel reason is invalid")
)
