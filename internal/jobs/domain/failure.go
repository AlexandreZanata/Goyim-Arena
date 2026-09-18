package domain

import "strings"

// FailureCode is the closed vocabulary of stable failure codes recorded on a
// job. It is deliberately small: an operator can group and alert on these
// without ever reading a provider message or a stack trace.
type FailureCode string

const (
	// FailureInvalidPayload is a payload the handler refused to interpret.
	FailureInvalidPayload FailureCode = "JOB_INVALID_PAYLOAD"
	// FailureUnknownType is a type with no registered handler.
	FailureUnknownType FailureCode = "JOB_UNKNOWN_TYPE"
	// FailureUnsupportedVersion is a payload version the handler cannot read.
	FailureUnsupportedVersion FailureCode = "JOB_UNSUPPORTED_VERSION"
	// FailureHandlerError is a handler that returned an error.
	FailureHandlerError FailureCode = "JOB_HANDLER_ERROR"
	// FailureHandlerTimeout is a handler that exceeded its deadline.
	FailureHandlerTimeout FailureCode = "JOB_HANDLER_TIMEOUT"
	// FailureLeaseExpired is work reclaimed because its lease ended.
	FailureLeaseExpired FailureCode = "JOB_LEASE_EXPIRED"
)

// AllFailureCodes lists the vocabulary in a deterministic order.
var AllFailureCodes = []FailureCode{
	FailureInvalidPayload,
	FailureUnknownType,
	FailureUnsupportedVersion,
	FailureHandlerError,
	FailureHandlerTimeout,
	FailureLeaseExpired,
}

// IsValid reports whether the code belongs to the closed vocabulary.
func (c FailureCode) IsValid() bool {
	for _, known := range AllFailureCodes {
		if c == known {
			return true
		}
	}
	return false
}

// Failure is the redacted record of why a job did not succeed: a stable code
// plus a bounded detail. It is the only shape the queue can store, so an
// arbitrary error string cannot reach the column.
type Failure struct {
	Code   FailureCode
	Detail string
}

// NewFailure builds a redacted failure. The code must be part of the closed
// vocabulary and the detail is sanitized: only letters, digits and the
// separators `_ - . : ,` and spaces survive, so URLs, credentials, query
// strings, email addresses, quotes and newlines are unrepresentable.
func NewFailure(code FailureCode, detail string) (Failure, error) {
	if !code.IsValid() {
		return Failure{}, ErrUnknownFailureCode
	}
	return Failure{Code: code, Detail: SanitizeDetail(detail)}, nil
}

// SanitizeDetail reduces arbitrary text to a bounded, harmless detail.
func SanitizeDetail(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '_' || r == '-' || r == '.' || r == ':' || r == ',':
			return r
		default:
			return -1
		}
	}, raw)

	// Collapse the runs of separators the mapping can leave behind.
	fields := strings.Fields(cleaned)
	cleaned = strings.Join(fields, " ")

	if len(cleaned) > MaxErrorDetailLength {
		cleaned = cleaned[:MaxErrorDetailLength]
	}
	if cleaned == "" {
		cleaned = "redacted"
	}
	return cleaned
}
