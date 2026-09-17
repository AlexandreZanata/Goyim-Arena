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
)
