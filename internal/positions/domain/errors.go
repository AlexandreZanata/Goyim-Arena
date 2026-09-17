package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule
// violations.
type ErrorCode string

const (
	CodeEmptyArenaID     ErrorCode = "POSITION_EMPTY_ARENA_ID"
	CodeInvalidArenaID   ErrorCode = "POSITION_INVALID_ARENA_ID"
	CodeEmptyAccountID   ErrorCode = "POSITION_EMPTY_ACCOUNT_ID"
	CodeInvalidAccountID ErrorCode = "POSITION_INVALID_ACCOUNT_ID"
	CodeEmptyPosition    ErrorCode = "POSITION_EMPTY"
	CodeInvalidPosition  ErrorCode = "POSITION_INVALID"
	CodeSamePosition     ErrorCode = "POSITION_SAME"
	CodeInvalidVersion   ErrorCode = "POSITION_INVALID_VERSION"
	CodeInvalidInstant   ErrorCode = "POSITION_INVALID_INSTANT"
	CodeBrokenChain      ErrorCode = "POSITION_BROKEN_CHAIN"
)

// DomainError represents an invariant or rule failure in the positions
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
	// ErrEmptyArenaID indicates a missing Arena identifier.
	ErrEmptyArenaID = DomainError{Code: CodeEmptyArenaID, Message: "arena identifier cannot be empty"}

	// ErrInvalidArenaID indicates a malformed Arena identifier.
	ErrInvalidArenaID = DomainError{Code: CodeInvalidArenaID, Message: "arena identifier format is invalid"}

	// ErrEmptyAccountID indicates a missing account identifier.
	ErrEmptyAccountID = DomainError{Code: CodeEmptyAccountID, Message: "account identifier cannot be empty"}

	// ErrInvalidAccountID indicates a malformed account identifier.
	ErrInvalidAccountID = DomainError{Code: CodeInvalidAccountID, Message: "account identifier format is invalid"}

	// ErrEmptyPosition indicates a missing position value.
	ErrEmptyPosition = DomainError{Code: CodeEmptyPosition, Message: "position cannot be empty"}

	// ErrInvalidPosition indicates a value outside the closed vocabulary:
	// agree, disagree or undecided.
	ErrInvalidPosition = DomainError{Code: CodeInvalidPosition, Message: "position must be agree, disagree or undecided"}

	// ErrSamePosition indicates a change targeting the current position.
	// Changing to the same value is not a change.
	ErrSamePosition = DomainError{Code: CodeSamePosition, Message: "a change must target a position different from the current one"}

	// ErrInvalidVersion indicates a version outside the valid range: at
	// least 1 for the projection, at least 2 for a change.
	ErrInvalidVersion = DomainError{Code: CodeInvalidVersion, Message: "position version is out of range"}

	// ErrInvalidInstant indicates a missing instant or one that moves the
	// history backwards.
	ErrInvalidInstant = DomainError{Code: CodeInvalidInstant, Message: "instant must be set and never precede the previous one"}

	// ErrBrokenChain indicates a change chain that is not contiguous: a
	// version gap or a from_position different from the running position.
	ErrBrokenChain = DomainError{Code: CodeBrokenChain, Message: "position change chain is not contiguous"}
)
