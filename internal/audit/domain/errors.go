package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyActorID      ErrorCode = "AUDIT_EMPTY_ACTOR_ID"
	CodeInvalidAction     ErrorCode = "AUDIT_INVALID_ACTION"
	CodeInvalidTarget     ErrorCode = "AUDIT_INVALID_TARGET"
	CodeEmptyReasonCode   ErrorCode = "AUDIT_EMPTY_REASON_CODE"
	CodeReasonCodeTooLong ErrorCode = "AUDIT_REASON_CODE_TOO_LONG"
	CodeInvalidMetadata   ErrorCode = "AUDIT_INVALID_METADATA"
	CodeEmptyOccurredAt   ErrorCode = "AUDIT_EMPTY_OCCURRED_AT"
)

// DomainError represents an invariant or rule failure in the audit domain.
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
	ErrEmptyActorID      = DomainError{Code: CodeEmptyActorID, Message: "audit actor identifier cannot be empty"}
	ErrInvalidAction     = DomainError{Code: CodeInvalidAction, Message: "audit action must be namespaced module.operation"}
	ErrInvalidTarget     = DomainError{Code: CodeInvalidTarget, Message: "audit target type or identifier is invalid"}
	ErrEmptyReasonCode   = DomainError{Code: CodeEmptyReasonCode, Message: "audit reason code cannot be empty"}
	ErrReasonCodeTooLong = DomainError{Code: CodeReasonCodeTooLong, Message: "audit reason code exceeds the maximum allowed length"}
	ErrInvalidMetadata   = DomainError{Code: CodeInvalidMetadata, Message: "audit metadata carries a forbidden key or an oversized value"}
	ErrEmptyOccurredAt   = DomainError{Code: CodeEmptyOccurredAt, Message: "audit instant cannot be empty"}
)
