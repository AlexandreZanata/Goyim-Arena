package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule
// violations.
type ErrorCode string

const (
	CodeEmptyContent             ErrorCode = "ARGUMENT_EMPTY_CONTENT"
	CodeInvalidContent           ErrorCode = "ARGUMENT_INVALID_CONTENT"
	CodeContentTooLong           ErrorCode = "ARGUMENT_CONTENT_TOO_LONG"
	CodeMissingGraphemeCounter   ErrorCode = "ARGUMENT_MISSING_GRAPHEME_COUNTER"
	CodeInvalidContentHash       ErrorCode = "ARGUMENT_INVALID_CONTENT_HASH"
	CodeContentHashMismatch      ErrorCode = "ARGUMENT_CONTENT_HASH_MISMATCH"
	CodeEmptyRelation            ErrorCode = "ARGUMENT_EMPTY_RELATION"
	CodeInvalidRelation          ErrorCode = "ARGUMENT_INVALID_RELATION"
	CodeEmptySourceURL           ErrorCode = "ARGUMENT_EMPTY_SOURCE_URL"
	CodeInvalidSourceURL         ErrorCode = "ARGUMENT_INVALID_SOURCE_URL"
	CodeSourceDescriptionTooLong ErrorCode = "ARGUMENT_SOURCE_DESCRIPTION_TOO_LONG"
)

// DomainError represents an invariant or rule failure in the arguments
// domain.
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
	// ErrEmptyContent indicates content that is empty or whitespace only
	// after normalization.
	ErrEmptyContent = DomainError{Code: CodeEmptyContent, Message: "argument content cannot be empty or whitespace only"}

	// ErrInvalidContent indicates content that is not valid UTF-8 or carries
	// control characters (newline excepted) or bidirectional overrides.
	ErrInvalidContent = DomainError{Code: CodeInvalidContent, Message: "argument content contains unsupported characters"}

	// ErrContentTooLong indicates content above the 3.000 grapheme cluster
	// limit.
	ErrContentTooLong = DomainError{Code: CodeContentTooLong, Message: "argument content exceeds 3000 grapheme clusters"}

	// ErrMissingGraphemeCounter indicates a missing grapheme counter. The
	// domain never counts clusters itself (ADR-013): the counter is a
	// required collaborator.
	ErrMissingGraphemeCounter = DomainError{Code: CodeMissingGraphemeCounter, Message: "a grapheme counter is required"}

	// ErrInvalidContentHash indicates a hash outside the versioned canonical
	// format.
	ErrInvalidContentHash = DomainError{Code: CodeInvalidContentHash, Message: "content hash format is invalid"}

	// ErrContentHashMismatch indicates stored content that does not hash to
	// its recorded canonical hash.
	ErrContentHashMismatch = DomainError{Code: CodeContentHashMismatch, Message: "content does not match its canonical hash"}

	// ErrEmptyRelation indicates a missing relation value.
	ErrEmptyRelation = DomainError{Code: CodeEmptyRelation, Message: "argument relation cannot be empty"}

	// ErrInvalidRelation indicates a relation outside the closed vocabulary:
	// support, oppose or context.
	ErrInvalidRelation = DomainError{Code: CodeInvalidRelation, Message: "argument relation must be support, oppose or context"}

	// ErrEmptySourceURL indicates a missing source URL.
	ErrEmptySourceURL = DomainError{Code: CodeEmptySourceURL, Message: "source URL cannot be empty"}

	// ErrInvalidSourceURL indicates a URL that is not an absolute http(s)
	// address within the structural bounds.
	ErrInvalidSourceURL = DomainError{Code: CodeInvalidSourceURL, Message: "source URL must be an absolute http(s) address"}

	// ErrSourceDescriptionTooLong indicates a description above the limit.
	ErrSourceDescriptionTooLong = DomainError{Code: CodeSourceDescriptionTooLong, Message: "source description exceeds 500 characters"}
)
