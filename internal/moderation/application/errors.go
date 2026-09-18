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

	// ErrCaseNotFound indicates no case carries the identifier. It is
	// distinct from a wrong-state case so a forged identifier is never
	// treated as a triage failure.
	ErrCaseNotFound = errors.New("application: moderation case was not found")

	// ErrCaseAlreadyClaimed indicates a live lease already owns the review.
	// The caller must wait for expiry or route elsewhere, never steal.
	ErrCaseAlreadyClaimed = errors.New("application: moderation case is already claimed under a live lease")

	// ErrLeaseExpired indicates the claimant's lease lapsed before the
	// decision was recorded. Reclaim first, then decide.
	ErrLeaseExpired = errors.New("application: moderation claim lease expired")

	// ErrInvalidCaseTransition indicates the case lifecycle forbids the
	// requested move (for example deciding an open case twice).
	ErrInvalidCaseTransition = errors.New("application: moderation case transition is invalid")

	// ErrInvalidDecisionConfig indicates the decision use cases could not
	// be built from the given dependencies.
	ErrInvalidDecisionConfig = errors.New("application: moderation decision configuration is invalid")

	// ErrActionNotFound indicates no action carries the identifier. It is
	// distinct from an ineligible action so a forged identifier is never
	// treated as a contestable sanction.
	ErrActionNotFound = errors.New("application: moderation action was not found")

	// ErrActionNotAppealable indicates the action sanctions nobody (for
	// example no_action), so there is no affected owner to appeal it.
	ErrActionNotAppealable = errors.New("application: moderation action cannot be appealed")

	// ErrNotAppealOwner indicates the appellant does not own the sanctioned
	// target. Only the affected user contests a sanction, never a third
	// party.
	ErrNotAppealOwner = errors.New("application: only the sanctioned owner may appeal")

	// ErrAppealExpired indicates the sanction settled past the appeal
	// window. Late appeals deny distinctly from duplicates.
	ErrAppealExpired = errors.New("application: appeal window expired")

	// ErrAppealDuplicate indicates the action already carries an appeal.
	// Exactly one appeal contests one action.
	ErrAppealDuplicate = errors.New("application: moderation action already carries an appeal")

	// ErrAppealNotFound indicates no appeal carries the identifier.
	ErrAppealNotFound = errors.New("application: moderation appeal was not found")

	// ErrAppealAlreadyClaimed indicates a live review already owns the
	// appeal. Concurrent reviewers serialize; only one claims.
	ErrAppealAlreadyClaimed = errors.New("application: moderation appeal is already under review")

	// ErrSameReviewer indicates the reviewer decided the original action.
	// Reviews belong to a different reviewer when one is available.
	ErrSameReviewer = errors.New("application: appeal reviewer must differ from the deciding moderator")

	// ErrInvalidAppealTransition indicates the appeal lifecycle forbids the
	// requested move.
	ErrInvalidAppealTransition = errors.New("application: moderation appeal transition is invalid")

	// ErrInvalidAppealConfig indicates the appeal use cases could not be
	// built from the given dependencies.
	ErrInvalidAppealConfig = errors.New("application: moderation appeal configuration is invalid")
)
