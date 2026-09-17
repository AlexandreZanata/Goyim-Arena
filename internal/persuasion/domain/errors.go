package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule
// violations.
type ErrorCode string

const (
	CodeInvalidPolicy           ErrorCode = "PERSUASION_INVALID_POLICY"
	CodeEmptyChangeID           ErrorCode = "PERSUASION_EMPTY_CHANGE_ID"
	CodeEmptyArgumentID         ErrorCode = "PERSUASION_EMPTY_ARGUMENT_ID"
	CodeEmptyArenaID            ErrorCode = "PERSUASION_EMPTY_ARENA_ID"
	CodeInvalidIdentifier       ErrorCode = "PERSUASION_INVALID_IDENTIFIER"
	CodeEmptyAttributorID       ErrorCode = "PERSUASION_EMPTY_ATTRIBUTOR_ID"
	CodeEmptyAuthorID           ErrorCode = "PERSUASION_EMPTY_AUTHOR_ID"
	CodeInvalidInstant          ErrorCode = "PERSUASION_INVALID_INSTANT"
	CodeInvalidStatus           ErrorCode = "PERSUASION_INVALID_STATUS"
	CodeTooManyAttributions     ErrorCode = "PERSUASION_TOO_MANY_ATTRIBUTIONS"
	CodeDuplicateAttribution    ErrorCode = "PERSUASION_DUPLICATE_ATTRIBUTION"
	CodeCrossArenaArgument      ErrorCode = "PERSUASION_CROSS_ARENA_ARGUMENT"
	CodeSelfAttribution         ErrorCode = "PERSUASION_SELF_ATTRIBUTION"
	CodeArgumentNotBeforeChange ErrorCode = "PERSUASION_ARGUMENT_NOT_BEFORE_CHANGE"
	CodeArgumentNotEligible     ErrorCode = "PERSUASION_ARGUMENT_NOT_ELIGIBLE"
)

// DomainError represents an invariant or rule failure in the persuasion
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
	// ErrInvalidPolicy indicates a malformed eligibility policy.
	ErrInvalidPolicy = DomainError{Code: CodeInvalidPolicy, Message: "attribution eligibility policy is invalid"}

	// ErrEmptyChangeID indicates a missing position change identifier.
	ErrEmptyChangeID = DomainError{Code: CodeEmptyChangeID, Message: "position change identifier cannot be empty"}

	// ErrEmptyArgumentID indicates a missing argument identifier.
	ErrEmptyArgumentID = DomainError{Code: CodeEmptyArgumentID, Message: "argument identifier cannot be empty"}

	// ErrEmptyArenaID indicates a missing Arena identifier.
	ErrEmptyArenaID = DomainError{Code: CodeEmptyArenaID, Message: "arena identifier cannot be empty"}

	// ErrInvalidIdentifier indicates a malformed opaque identifier.
	ErrInvalidIdentifier = DomainError{Code: CodeInvalidIdentifier, Message: "identifier format is invalid"}

	// ErrEmptyAttributorID indicates a missing attributor identifier.
	ErrEmptyAttributorID = DomainError{Code: CodeEmptyAttributorID, Message: "attributor identifier cannot be empty"}

	// ErrEmptyAuthorID indicates a missing argument author identifier.
	ErrEmptyAuthorID = DomainError{Code: CodeEmptyAuthorID, Message: "argument author identifier cannot be empty"}

	// ErrInvalidInstant indicates a missing change instant.
	ErrInvalidInstant = DomainError{Code: CodeInvalidInstant, Message: "position change instant must be set"}

	// ErrInvalidStatus indicates an argument status outside the closed
	// vocabulary.
	ErrInvalidStatus = DomainError{Code: CodeInvalidStatus, Message: "argument status must be published, withdrawn or removed"}

	// ErrTooManyAttributions indicates a selection above the policy limit.
	ErrTooManyAttributions = DomainError{Code: CodeTooManyAttributions, Message: "a change credits at most three arguments"}

	// ErrDuplicateAttribution indicates the same argument twice in one
	// selection: each argument counts at most once per change.
	ErrDuplicateAttribution = DomainError{Code: CodeDuplicateAttribution, Message: "the same argument cannot be credited twice by one change"}

	// ErrCrossArenaArgument indicates an argument from another Arena.
	ErrCrossArenaArgument = DomainError{Code: CodeCrossArenaArgument, Message: "the argument belongs to another arena"}

	// ErrSelfAttribution indicates the attributor crediting their own
	// argument.
	ErrSelfAttribution = DomainError{Code: CodeSelfAttribution, Message: "an argument cannot be credited by its own author"}

	// ErrArgumentNotBeforeChange indicates an argument published after the
	// change (or at the same instant).
	ErrArgumentNotBeforeChange = DomainError{Code: CodeArgumentNotBeforeChange, Message: "the argument must have been published before the change"}

	// ErrArgumentNotEligible indicates an argument unavailable to the
	// public (withdrawn or removed) at selection time.
	ErrArgumentNotEligible = DomainError{Code: CodeArgumentNotEligible, Message: "the argument is not eligible for attribution"}
)
