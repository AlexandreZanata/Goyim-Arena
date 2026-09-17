package domain

import "fmt"

// ErrorCode is a stable machine-readable identifier for domain rule violations.
type ErrorCode string

const (
	CodeEmptyArenaID        ErrorCode = "ARENA_EMPTY_ID"
	CodeEmptyCreatorID      ErrorCode = "ARENA_EMPTY_CREATOR_ID"
	CodeInvalidVersion      ErrorCode = "ARENA_INVALID_VERSION"
	CodeInvalidPolicy       ErrorCode = "ARENA_INVALID_POLICY"
	CodeEmptyStatement      ErrorCode = "ARENA_EMPTY_STATEMENT"
	CodeStatementTooShort   ErrorCode = "ARENA_STATEMENT_TOO_SHORT"
	CodeStatementTooLong    ErrorCode = "ARENA_STATEMENT_TOO_LONG"
	CodeInvalidStatement    ErrorCode = "ARENA_INVALID_STATEMENT"
	CodeContextTooLong      ErrorCode = "ARENA_CONTEXT_TOO_LONG"
	CodeInvalidContext      ErrorCode = "ARENA_INVALID_CONTEXT"
	CodeEmptySlug           ErrorCode = "ARENA_EMPTY_SLUG"
	CodeInvalidSlug         ErrorCode = "ARENA_INVALID_SLUG"
	CodeEmptyLanguage       ErrorCode = "ARENA_EMPTY_LANGUAGE"
	CodeInvalidLanguage     ErrorCode = "ARENA_INVALID_LANGUAGE"
	CodeUnsupportedLanguage ErrorCode = "ARENA_UNSUPPORTED_LANGUAGE"
	CodeEmptyCategory       ErrorCode = "ARENA_EMPTY_CATEGORY"
	CodeInvalidCategory     ErrorCode = "ARENA_INVALID_CATEGORY"
	CodeInvalidStatus       ErrorCode = "ARENA_INVALID_STATUS"
	CodeInvalidStatusChange ErrorCode = "ARENA_INVALID_STATUS_TRANSITION"
	CodeArenaNotDraft       ErrorCode = "ARENA_NOT_DRAFT"
	CodeMissingSlug         ErrorCode = "ARENA_MISSING_SLUG"
	CodeInvalidCloseDate    ErrorCode = "ARENA_INVALID_CLOSE_DATE"
	CodeArenaNotOpen        ErrorCode = "ARENA_NOT_OPEN"
	CodeActorRequired       ErrorCode = "ARENA_ACTOR_REQUIRED"
	CodeEmptyReason         ErrorCode = "ARENA_EMPTY_REASON"
	CodeInvalidReason       ErrorCode = "ARENA_INVALID_REASON"
	CodeReasonTooLong       ErrorCode = "ARENA_REASON_TOO_LONG"
)

// DomainError represents an invariant or rule failure in the arenas domain.
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
	ErrEmptyArenaID        = DomainError{Code: CodeEmptyArenaID, Message: "arena identifier cannot be empty"}
	ErrEmptyCreatorID      = DomainError{Code: CodeEmptyCreatorID, Message: "arena creator identifier cannot be empty"}
	ErrInvalidVersion      = DomainError{Code: CodeInvalidVersion, Message: "arena version must be a positive integer"}
	ErrInvalidPolicy       = DomainError{Code: CodeInvalidPolicy, Message: "arena text policy is invalid"}
	ErrEmptyStatement      = DomainError{Code: CodeEmptyStatement, Message: "arena statement cannot be empty"}
	ErrStatementTooShort   = DomainError{Code: CodeStatementTooShort, Message: "arena statement is shorter than the configured minimum"}
	ErrStatementTooLong    = DomainError{Code: CodeStatementTooLong, Message: "arena statement exceeds the configured maximum"}
	ErrInvalidStatement    = DomainError{Code: CodeInvalidStatement, Message: "arena statement contains unsupported characters"}
	ErrContextTooLong      = DomainError{Code: CodeContextTooLong, Message: "arena context exceeds the configured maximum"}
	ErrInvalidContext      = DomainError{Code: CodeInvalidContext, Message: "arena context contains unsupported characters"}
	ErrEmptySlug           = DomainError{Code: CodeEmptySlug, Message: "arena slug cannot be empty"}
	ErrInvalidSlug         = DomainError{Code: CodeInvalidSlug, Message: "arena slug format is invalid"}
	ErrEmptyLanguage       = DomainError{Code: CodeEmptyLanguage, Message: "arena language cannot be empty"}
	ErrInvalidLanguage     = DomainError{Code: CodeInvalidLanguage, Message: "arena language is not a well-formed BCP 47 tag"}
	ErrUnsupportedLanguage = DomainError{Code: CodeUnsupportedLanguage, Message: "arena language is not supported by the product"}
	ErrEmptyCategory       = DomainError{Code: CodeEmptyCategory, Message: "arena category cannot be empty"}
	ErrInvalidCategory     = DomainError{Code: CodeInvalidCategory, Message: "arena category format is invalid"}
	ErrInvalidStatus       = DomainError{Code: CodeInvalidStatus, Message: "arena status is unrecognized"}
	ErrInvalidStatusChange = DomainError{Code: CodeInvalidStatusChange, Message: "arena status transition is not permitted"}
	ErrArenaNotDraft       = DomainError{Code: CodeArenaNotDraft, Message: "arena is not a draft"}
	ErrMissingSlug         = DomainError{Code: CodeMissingSlug, Message: "publishing an arena requires a slug"}
	ErrInvalidCloseDate    = DomainError{Code: CodeInvalidCloseDate, Message: "arena close date must be after its publication"}
	ErrArenaNotOpen        = DomainError{Code: CodeArenaNotOpen, Message: "arena is not open to participation"}
	ErrActorRequired       = DomainError{Code: CodeActorRequired, Message: "moderation actions require an acting moderator"}
	ErrEmptyReason         = DomainError{Code: CodeEmptyReason, Message: "moderation actions require a reason"}
	ErrInvalidReason       = DomainError{Code: CodeInvalidReason, Message: "reason contains unsupported characters"}
	ErrReasonTooLong       = DomainError{Code: CodeReasonTooLong, Message: "reason exceeds the maximum allowed length"}
)
