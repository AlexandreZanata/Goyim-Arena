package application

import "errors"

var (
	// ErrChangeNotFound indicates the position change does not exist or
	// belongs to another account: foreign changes are deliberately
	// indistinguishable from missing ones (owner-only rule).
	ErrChangeNotFound = errors.New("persuasion: position change not found")

	// ErrArgumentNotFound indicates one of the proposed arguments does not
	// exist.
	ErrArgumentNotFound = errors.New("persuasion: argument not found")

	// ErrAttributionNotFound indicates the attribution does not exist.
	ErrAttributionNotFound = errors.New("persuasion: attribution not found")

	// ErrNotAuthorized indicates the acting account is not allowed to
	// moderate attributions.
	ErrNotAuthorized = errors.New("persuasion: actor is not authorized for attribution moderation")

	// ErrModerationConflict indicates a concurrent decision changed the
	// validity between the locked read and the guarded write, so the decision
	// was not applied. Callers re-read and retry.
	ErrModerationConflict = errors.New("persuasion: attribution moderation conflict")

	// ErrInvalidReputationProjection indicates an incoherent reputation
	// projection: missing identity, repeated Arena, missing dimension label,
	// negative count or more people than events. Publishing it would silently
	// inflate public reputation, so it is refused instead.
	ErrInvalidReputationProjection = errors.New("persuasion: reputation projection is incoherent")

	// ErrInvalidAuthorID indicates the author identifier cannot address an
	// account at all, so no reputation fact can be derived from it. An
	// identifier that addresses an author without valid attributions derives
	// zeroed facts instead of this error: existence belongs to the profile
	// layer, not to the metric.
	ErrInvalidAuthorID = errors.New("persuasion: author identifier is invalid")

	// ErrProfileNotFound indicates the public username does not belong to any
	// profile, so no reputation can be addressed by it. It carries the same
	// meaning as the profiles module's own lookup failure — a handle that does
	// not exist is absent, never an empty author.
	ErrProfileNotFound = errors.New("persuasion: profile not found")
)
