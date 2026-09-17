package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyAccountID        ErrorCode = "WALLET_EMPTY_ACCOUNT_ID"
	CodeEmptyOperationID      ErrorCode = "WALLET_EMPTY_OPERATION_ID"
	CodeNegativeInk           ErrorCode = "WALLET_NEGATIVE_INK"
	CodeInvalidInk            ErrorCode = "WALLET_INVALID_INK"
	CodeInkOverflow           ErrorCode = "WALLET_INK_OVERFLOW"
	CodeInsufficientInk       ErrorCode = "WALLET_INSUFFICIENT_INK"
	CodeZeroAmount            ErrorCode = "WALLET_ZERO_AMOUNT"
	CodeInvalidDirection      ErrorCode = "WALLET_INVALID_DIRECTION"
	CodeInvalidBucket         ErrorCode = "WALLET_INVALID_BUCKET"
	CodeInvalidOperationType  ErrorCode = "WALLET_INVALID_OPERATION_TYPE"
	CodeNotACredit            ErrorCode = "WALLET_NOT_A_CREDIT"
	CodeNotADebit             ErrorCode = "WALLET_NOT_A_DEBIT"
	CodeEmptyReference        ErrorCode = "WALLET_EMPTY_REFERENCE"
	CodeInvalidReference      ErrorCode = "WALLET_INVALID_REFERENCE"
	CodeReferenceTooLong      ErrorCode = "WALLET_REFERENCE_TOO_LONG"
	CodeEmptyIdempotencyKey   ErrorCode = "WALLET_EMPTY_IDEMPOTENCY_KEY"
	CodeInvalidIdempotencyKey ErrorCode = "WALLET_INVALID_IDEMPOTENCY_KEY"
	CodeIdempotencyKeyTooLong ErrorCode = "WALLET_IDEMPOTENCY_KEY_TOO_LONG"
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
	ErrEmptyAccountID        = DomainError{Code: CodeEmptyAccountID, Message: "account identifier cannot be empty"}
	ErrEmptyOperationID      = DomainError{Code: CodeEmptyOperationID, Message: "operation identifier cannot be empty"}
	ErrNegativeInk           = DomainError{Code: CodeNegativeInk, Message: "ink quantity cannot be negative"}
	ErrInvalidInk            = DomainError{Code: CodeInvalidInk, Message: "ink quantity is not a decimal integer"}
	ErrInkOverflow           = DomainError{Code: CodeInkOverflow, Message: "ink arithmetic would overflow the 64-bit range"}
	ErrInsufficientInk       = DomainError{Code: CodeInsufficientInk, Message: "ink subtraction would produce a negative quantity"}
	ErrZeroAmount            = DomainError{Code: CodeZeroAmount, Message: "ledger transactions require a non-zero amount"}
	ErrInvalidDirection      = DomainError{Code: CodeInvalidDirection, Message: "ledger direction is unrecognized"}
	ErrInvalidBucket         = DomainError{Code: CodeInvalidBucket, Message: "ink bucket is unrecognized"}
	ErrInvalidOperationType  = DomainError{Code: CodeInvalidOperationType, Message: "ink operation type is unrecognized"}
	ErrNotACredit            = DomainError{Code: CodeNotACredit, Message: "operation type is not a credit"}
	ErrNotADebit             = DomainError{Code: CodeNotADebit, Message: "operation type is not a debit"}
	ErrEmptyReference        = DomainError{Code: CodeEmptyReference, Message: "operation reference cannot be empty"}
	ErrInvalidReference      = DomainError{Code: CodeInvalidReference, Message: "operation reference contains unsupported characters"}
	ErrReferenceTooLong      = DomainError{Code: CodeReferenceTooLong, Message: "operation reference exceeds the maximum allowed length"}
	ErrEmptyIdempotencyKey   = DomainError{Code: CodeEmptyIdempotencyKey, Message: "idempotency key cannot be empty"}
	ErrInvalidIdempotencyKey = DomainError{Code: CodeInvalidIdempotencyKey, Message: "idempotency key contains unsupported characters"}
	ErrIdempotencyKeyTooLong = DomainError{Code: CodeIdempotencyKeyTooLong, Message: "idempotency key exceeds the maximum allowed length"}
)
