package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyAccountID     ErrorCode = "MODERATION_EMPTY_ACCOUNT_ID"
	CodeInvalidRole        ErrorCode = "MODERATION_INVALID_ROLE"
	CodeInvalidAction      ErrorCode = "MODERATION_INVALID_ACTION"
	CodeRoleNotAuthorized  ErrorCode = "MODERATION_ROLE_NOT_AUTHORIZED"
	CodeConflictOfInterest ErrorCode = "MODERATION_CONFLICT_OF_INTEREST"
	CodeStepUpRequired     ErrorCode = "MODERATION_STEP_UP_REQUIRED"
	CodeRoleRevoked        ErrorCode = "MODERATION_ROLE_REVOKED"
	CodeInvalidSessionAge  ErrorCode = "MODERATION_INVALID_SESSION_AGE"
)

// DomainError represents an invariant or rule failure in the moderation domain.
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
	ErrEmptyAccountID     = DomainError{Code: CodeEmptyAccountID, Message: "account identifier cannot be empty"}
	ErrInvalidRole        = DomainError{Code: CodeInvalidRole, Message: "administrative role is unrecognized"}
	ErrInvalidAction      = DomainError{Code: CodeInvalidAction, Message: "moderation action is unrecognized"}
	ErrRoleNotAuthorized  = DomainError{Code: CodeRoleNotAuthorized, Message: "role may not take this moderation action"}
	ErrConflictOfInterest = DomainError{Code: CodeConflictOfInterest, Message: "actor must declare conflict and pass the case to another reviewer"}
	ErrStepUpRequired     = DomainError{Code: CodeStepUpRequired, Message: "sensitive action requires recent authentication"}
	ErrRoleRevoked        = DomainError{Code: CodeRoleRevoked, Message: "administrative assignment is revoked"}
	ErrInvalidSessionAge  = DomainError{Code: CodeInvalidSessionAge, Message: "session age is incoherent"}
)
