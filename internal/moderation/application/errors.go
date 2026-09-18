package application

import "errors"

var (
	// ErrNotAuthorized indicates the actor holds no active assignment for
	// the requested action. It is distinct from a revoked assignment so a
	// forged identifier is never treated as a lapsed moderator.
	ErrNotAuthorized = errors.New("application: account is not authorized for this moderation action")

	// ErrRoleRevoked indicates the assignment exists but was revoked. A
	// revoked role in an otherwise active session still denies: session
	// validity never implies capability.
	ErrRoleRevoked = errors.New("application: administrative assignment is revoked")

	// ErrConflictOfInterest indicates the actor is involved with the target
	// and must pass the case to another reviewer.
	ErrConflictOfInterest = errors.New("application: actor must declare conflict and pass the case to another reviewer")

	// ErrStepUpRequired indicates the session authentication is too old
	// for a high-impact action and the actor must re-authenticate.
	ErrStepUpRequired = errors.New("application: sensitive action requires recent authentication")

	// ErrInvalidAuthorizerConfig indicates the authorizer could not be built
	// from the given dependencies.
	ErrInvalidAuthorizerConfig = errors.New("application: moderation authorizer configuration is invalid")

	// ErrTargetNotFound indicates the contested target does not exist. It
	// is distinct from a removed target so a forged identifier is never
	// treated as moderated content.
	ErrTargetNotFound = errors.New("application: report target was not found")

	// ErrTargetRemoved indicates the contested target is already gone
	// (removed or withdrawn). There is nothing left to moderate.
	ErrTargetRemoved = errors.New("application: report target is already removed")

	// ErrInvalidReportConfig indicates the report use case could not be
	// built from the given dependencies.
	ErrInvalidReportConfig = errors.New("application: moderation report configuration is invalid")
)
