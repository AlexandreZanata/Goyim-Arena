package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyEmail               ErrorCode = "AUTH_EMPTY_EMAIL"
	CodeInvalidEmail             ErrorCode = "AUTH_INVALID_EMAIL"
	CodeEmailTooLong             ErrorCode = "AUTH_EMAIL_TOO_LONG"
	CodeEmptyAccountID           ErrorCode = "AUTH_EMPTY_ACCOUNT_ID"
	CodeInvalidAccountStatus     ErrorCode = "AUTH_INVALID_ACCOUNT_STATUS"
	CodeInvalidAccountTransition ErrorCode = "AUTH_INVALID_ACCOUNT_TRANSITION"
	CodeAccountSuspended         ErrorCode = "AUTH_ACCOUNT_SUSPENDED"
	CodeAccountDeleted           ErrorCode = "AUTH_ACCOUNT_DELETED"
	CodeAccountAlreadyVerified   ErrorCode = "AUTH_ACCOUNT_ALREADY_VERIFIED"
	CodeAccountNotActive         ErrorCode = "AUTH_ACCOUNT_NOT_ACTIVE"
)

// DomainError represents an invariant or rule failure in the identity domain.
type DomainError struct {
	Code    ErrorCode
	Message string
}

func (e DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e DomainError) Is(target error) bool {
	t, ok := target.(DomainError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

var (
	ErrEmptyEmail               = DomainError{Code: CodeEmptyEmail, Message: "email address cannot be empty"}
	ErrInvalidEmail             = DomainError{Code: CodeInvalidEmail, Message: "email address syntax or character set is invalid"}
	ErrEmailTooLong             = DomainError{Code: CodeEmailTooLong, Message: "email address exceeds maximum allowed length of 254 characters"}
	ErrEmptyAccountID           = DomainError{Code: CodeEmptyAccountID, Message: "account identifier cannot be empty"}
	ErrInvalidAccountStatus     = DomainError{Code: CodeInvalidAccountStatus, Message: "account status is unrecognized"}
	ErrInvalidAccountTransition = DomainError{Code: CodeInvalidAccountTransition, Message: "account status transition is not permitted"}
	ErrAccountSuspended         = DomainError{Code: CodeAccountSuspended, Message: "account is suspended"}
	ErrAccountDeleted           = DomainError{Code: CodeAccountDeleted, Message: "account is deleted"}
	ErrAccountAlreadyVerified   = DomainError{Code: CodeAccountAlreadyVerified, Message: "account email is already verified"}
	ErrAccountNotActive         = DomainError{Code: CodeAccountNotActive, Message: "account is not in active state"}
)
