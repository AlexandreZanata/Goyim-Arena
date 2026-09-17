package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeNegativeInk          ErrorCode = "WALLET_NEGATIVE_INK"
	CodeInvalidInk           ErrorCode = "WALLET_INVALID_INK"
	CodeInkOverflow          ErrorCode = "WALLET_INK_OVERFLOW"
	CodeInsufficientInk      ErrorCode = "WALLET_INSUFFICIENT_INK"
	CodeZeroAmount           ErrorCode = "WALLET_ZERO_AMOUNT"
	CodeInvalidDirection     ErrorCode = "WALLET_INVALID_DIRECTION"
	CodeInvalidBucket        ErrorCode = "WALLET_INVALID_BUCKET"
	CodeInvalidOperationType ErrorCode = "WALLET_INVALID_OPERATION_TYPE"
	CodeEmptyReference       ErrorCode = "WALLET_EMPTY_REFERENCE"
	CodeInvalidReference     ErrorCode = "WALLET_INVALID_REFERENCE"
	CodeReferenceTooLong     ErrorCode = "WALLET_REFERENCE_TOO_LONG"
)

// DomainError represents an invariant or rule failure in the wallet domain.
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
	ErrNegativeInk          = DomainError{Code: CodeNegativeInk, Message: "ink quantity cannot be negative"}
	ErrInvalidInk           = DomainError{Code: CodeInvalidInk, Message: "ink quantity is not a decimal integer"}
	ErrInkOverflow          = DomainError{Code: CodeInkOverflow, Message: "ink arithmetic would overflow the 64-bit range"}
	ErrInsufficientInk      = DomainError{Code: CodeInsufficientInk, Message: "ink subtraction would produce a negative quantity"}
	ErrZeroAmount           = DomainError{Code: CodeZeroAmount, Message: "ledger transactions require a non-zero amount"}
	ErrInvalidDirection     = DomainError{Code: CodeInvalidDirection, Message: "ledger direction is unrecognized"}
	ErrInvalidBucket        = DomainError{Code: CodeInvalidBucket, Message: "ink bucket is unrecognized"}
	ErrInvalidOperationType = DomainError{Code: CodeInvalidOperationType, Message: "ink operation type is unrecognized"}
	ErrEmptyReference       = DomainError{Code: CodeEmptyReference, Message: "operation reference cannot be empty"}
	ErrInvalidReference     = DomainError{Code: CodeInvalidReference, Message: "operation reference contains unsupported characters"}
	ErrReferenceTooLong     = DomainError{Code: CodeReferenceTooLong, Message: "operation reference exceeds the maximum allowed length"}
)
