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

	// ErrReauthFailed indicates that a critical action was refused because
	// the caller did not re-authenticate. It covers a wrong password and a
	// missing credential alike: both are one refusal, and neither reveals
	// which one happened.
	ErrReauthFailed = errors.New("application: reauthentication failed")

	// ErrReauthUnavailable indicates that re-authentication cannot be
	// evaluated at all, which is a deployment fault and not a refusal of the
	// caller. The action fails closed either way.
	ErrReauthUnavailable = errors.New("application: reauthentication unavailable")

	// ErrCannotRevokeCurrentSession indicates that the caller addressed its
	// own session. Ending it is logging out, which has its own route.
	ErrCannotRevokeCurrentSession = errors.New("application: the current session is ended by logging out")

	// ErrMissingEmailSender indicates that a flow that must notify the owner
	// was composed without a sender. It is a wiring fault that fails closed:
	// a password change nobody is told about is the notification the owner
	// needed most.
	ErrMissingEmailSender = errors.New("application: email sender is required for this flow")
)
