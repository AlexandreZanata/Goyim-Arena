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

	CodeEmptyAttributionID        ErrorCode = "PERSUASION_EMPTY_ATTRIBUTION_ID"
	CodeEmptyModeratorID          ErrorCode = "PERSUASION_EMPTY_MODERATOR_ID"
	CodeInvalidAttributionStatus  ErrorCode = "PERSUASION_INVALID_ATTRIBUTION_STATUS"
	CodeInvalidModerationAction   ErrorCode = "PERSUASION_INVALID_MODERATION_ACTION"
	CodeEmptyReason               ErrorCode = "PERSUASION_EMPTY_REASON"
	CodeInvalidReason             ErrorCode = "PERSUASION_INVALID_REASON"
	CodeReasonTooLong             ErrorCode = "PERSUASION_REASON_TOO_LONG"
	CodeAttributionNotModeratable ErrorCode = "PERSUASION_ATTRIBUTION_NOT_MODERATABLE"
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

	// ErrEmptyAttributionID indicates a missing attribution identifier.
	ErrEmptyAttributionID = DomainError{Code: CodeEmptyAttributionID, Message: "attribution identifier cannot be empty"}

	// ErrEmptyModeratorID indicates a moderation decision without an acting
	// moderator.
	ErrEmptyModeratorID = DomainError{Code: CodeEmptyModeratorID, Message: "moderation decisions require an acting moderator"}

	// ErrInvalidAttributionStatus indicates a validity outside the closed
	// vocabulary valid|invalid.
	ErrInvalidAttributionStatus = DomainError{Code: CodeInvalidAttributionStatus, Message: "attribution status must be valid or invalid"}

	// ErrInvalidModerationAction indicates an action outside the closed
	// vocabulary or one that does not match the requested transition.
	ErrInvalidModerationAction = DomainError{Code: CodeInvalidModerationAction, Message: "moderation action must be invalidate or restore"}

	// ErrEmptyReason indicates a moderation decision without a reason.
	ErrEmptyReason = DomainError{Code: CodeEmptyReason, Message: "moderation decisions require a reason"}

	// ErrInvalidReason indicates a reason with unsupported characters.
	ErrInvalidReason = DomainError{Code: CodeInvalidReason, Message: "reason contains unsupported characters"}

	// ErrReasonTooLong indicates a reason above the configured maximum.
	ErrReasonTooLong = DomainError{Code: CodeReasonTooLong, Message: "reason exceeds the maximum allowed length"}

	// ErrAttributionNotModeratable indicates a transition that the current
	// validity already satisfies (invalidating an invalid attribution or
	// restoring a valid one).
	ErrAttributionNotModeratable = DomainError{Code: CodeAttributionNotModeratable, Message: "the attribution is already in the requested validity state"}
)
