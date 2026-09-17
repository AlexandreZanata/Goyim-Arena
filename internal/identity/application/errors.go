package application

import "errors"

var (
	// ErrDuplicateEmail indicates an account with this email address already exists.
	ErrDuplicateEmail = errors.New("application: account with this email address already exists")

	// ErrAccountNotFound indicates that the requested account was not found.
	ErrAccountNotFound = errors.New("application: account not found")

	// ErrInvalidToken indicates the verification token is invalid or does not exist.
	ErrInvalidToken = errors.New("application: verification token is invalid")

	// ErrTokenExpired indicates that the verification token lifetime has expired.
	ErrTokenExpired = errors.New("application: verification token has expired")

	// ErrTokenAlreadyUsed indicates that the verification token was already consumed or invalidated.
	ErrTokenAlreadyUsed = errors.New("application: verification token has already been used")

	// ErrWeakPassword indicates the provided password does not meet minimum strength requirements.
	ErrWeakPassword = errors.New("application: password must be at least 8 characters long")

	// ErrInvalidCredentials indicates that the provided email/password combination is invalid.
	ErrInvalidCredentials = errors.New("application: invalid email or password")

	// ErrCredentialNotFound indicates that no password credential was found for the account.
	ErrCredentialNotFound = errors.New("application: credential not found")

	// ErrInvalidSession indicates the session token is empty or malformed.
	ErrInvalidSession = errors.New("application: invalid session token")

	// ErrSessionNotFound indicates that the requested session does not exist.
	ErrSessionNotFound = errors.New("application: session not found")

	// ErrSessionRevoked indicates that the session has been revoked.
	ErrSessionRevoked = errors.New("application: session has been revoked")

	// ErrSessionExpired indicates that the session has expired.
	ErrSessionExpired = errors.New("application: session has expired")
)
