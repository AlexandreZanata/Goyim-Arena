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
)
