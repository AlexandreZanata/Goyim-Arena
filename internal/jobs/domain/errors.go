package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule failures.
type ErrorCode string

const (
	CodeEmptyJobID            ErrorCode = "JOB_EMPTY_ID"
	CodeUnknownJobType        ErrorCode = "JOB_UNKNOWN_TYPE"
	CodeInvalidVersion        ErrorCode = "JOB_INVALID_VERSION"
	CodeEmptyPayload          ErrorCode = "JOB_EMPTY_PAYLOAD"
	CodePayloadTooLarge       ErrorCode = "JOB_PAYLOAD_TOO_LARGE"
	CodePayloadNotObject      ErrorCode = "JOB_PAYLOAD_NOT_OBJECT"
	CodePayloadMalformed      ErrorCode = "JOB_PAYLOAD_MALFORMED"
	CodePayloadTooDeep        ErrorCode = "JOB_PAYLOAD_TOO_DEEP"
	CodeUnknownJobState       ErrorCode = "JOB_UNKNOWN_STATE"
	CodeInvalidAttempts       ErrorCode = "JOB_INVALID_ATTEMPTS"
	CodeInvalidLease          ErrorCode = "JOB_INVALID_LEASE"
	CodeLeaseIncoherent       ErrorCode = "JOB_LEASE_INCOHERENT"
	CodeInvalidIdempotencyKey ErrorCode = "JOB_INVALID_IDEMPOTENCY_KEY"
	CodeInvalidFailure        ErrorCode = "JOB_INVALID_FAILURE"
	CodeUnknownFailureCode    ErrorCode = "JOB_UNKNOWN_FAILURE_CODE"
	CodeInvalidTimestamps     ErrorCode = "JOB_INVALID_TIMESTAMPS"
)

// DomainError is an invariant or rule failure of the jobs domain.
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
	ErrEmptyJobID            = DomainError{Code: CodeEmptyJobID, Message: "job identifier cannot be empty"}
	ErrUnknownJobType        = DomainError{Code: CodeUnknownJobType, Message: "job type is not part of the closed workload vocabulary"}
	ErrInvalidVersion        = DomainError{Code: CodeInvalidVersion, Message: "payload version must be at least 1"}
	ErrEmptyPayload          = DomainError{Code: CodeEmptyPayload, Message: "payload cannot be empty"}
	ErrPayloadTooLarge       = DomainError{Code: CodePayloadTooLarge, Message: "payload exceeds the maximum allowed size"}
	ErrPayloadNotObject      = DomainError{Code: CodePayloadNotObject, Message: "payload must be a JSON object"}
	ErrPayloadMalformed      = DomainError{Code: CodePayloadMalformed, Message: "payload is not well-formed JSON"}
	ErrPayloadTooDeep        = DomainError{Code: CodePayloadTooDeep, Message: "payload nests deeper than the allowed structure"}
	ErrUnknownJobState       = DomainError{Code: CodeUnknownJobState, Message: "job state is not part of the lifecycle vocabulary"}
	ErrInvalidAttempts       = DomainError{Code: CodeInvalidAttempts, Message: "attempt counters are incoherent"}
	ErrInvalidLease          = DomainError{Code: CodeInvalidLease, Message: "lease holder or expiry is invalid"}
	ErrLeaseIncoherent       = DomainError{Code: CodeLeaseIncoherent, Message: "lease does not match the job state"}
	ErrInvalidIdempotencyKey = DomainError{Code: CodeInvalidIdempotencyKey, Message: "idempotency key is empty or too long"}
	ErrInvalidFailure        = DomainError{Code: CodeInvalidFailure, Message: "failure code and detail are incoherent"}
	ErrUnknownFailureCode    = DomainError{Code: CodeUnknownFailureCode, Message: "failure code is not part of the closed vocabulary"}
	ErrInvalidTimestamps     = DomainError{Code: CodeInvalidTimestamps, Message: "job timestamps are incoherent"}
)
