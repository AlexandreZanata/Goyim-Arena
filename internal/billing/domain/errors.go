package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyAccountID      ErrorCode = "BILLING_EMPTY_ACCOUNT_ID"
	CodeEmptyLotID          ErrorCode = "BILLING_EMPTY_LOT_ID"
	CodeInvalidPassOrigin   ErrorCode = "BILLING_INVALID_PASS_ORIGIN"
	CodeInvalidQuantity     ErrorCode = "BILLING_INVALID_QUANTITY"
	CodeInvalidRemaining    ErrorCode = "BILLING_INVALID_REMAINING"
	CodeEmptyReference      ErrorCode = "BILLING_EMPTY_REFERENCE"
	CodeInvalidReference    ErrorCode = "BILLING_INVALID_REFERENCE"
	CodeReferenceTooLong    ErrorCode = "BILLING_REFERENCE_TOO_LONG"
	CodeExpirationRequired  ErrorCode = "BILLING_EXPIRATION_REQUIRED"
	CodeExpirationForbidden ErrorCode = "BILLING_EXPIRATION_FORBIDDEN"
	CodeEmptyArenaID        ErrorCode = "BILLING_EMPTY_ARENA_ID"
	CodeInvalidArenaID      ErrorCode = "BILLING_INVALID_ARENA_ID"
	CodeNoPassAvailable     ErrorCode = "BILLING_NO_PASS_AVAILABLE"
)

// DomainError represents an invariant or rule failure in the billing domain.
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
	ErrEmptyAccountID      = DomainError{Code: CodeEmptyAccountID, Message: "account identifier cannot be empty"}
	ErrEmptyLotID          = DomainError{Code: CodeEmptyLotID, Message: "pass lot identifier cannot be empty"}
	ErrInvalidPassOrigin   = DomainError{Code: CodeInvalidPassOrigin, Message: "pass origin is unrecognized"}
	ErrInvalidQuantity     = DomainError{Code: CodeInvalidQuantity, Message: "pass quantity must be a positive integer"}
	ErrInvalidRemaining    = DomainError{Code: CodeInvalidRemaining, Message: "remaining passes must stay between zero and the granted quantity"}
	ErrEmptyReference      = DomainError{Code: CodeEmptyReference, Message: "grant reference cannot be empty"}
	ErrInvalidReference    = DomainError{Code: CodeInvalidReference, Message: "grant reference contains unsupported characters"}
	ErrReferenceTooLong    = DomainError{Code: CodeReferenceTooLong, Message: "grant reference exceeds the maximum allowed length"}
	ErrExpirationRequired  = DomainError{Code: CodeExpirationRequired, Message: "this pass origin requires an expiration"}
	ErrExpirationForbidden = DomainError{Code: CodeExpirationForbidden, Message: "this pass origin does not expire"}
	ErrEmptyArenaID        = DomainError{Code: CodeEmptyArenaID, Message: "arena identifier cannot be empty"}
	ErrInvalidArenaID      = DomainError{Code: CodeInvalidArenaID, Message: "arena identifier contains unsupported characters"}
	ErrNoPassAvailable     = DomainError{Code: CodeNoPassAvailable, Message: "no valid arena pass is available"}
)
