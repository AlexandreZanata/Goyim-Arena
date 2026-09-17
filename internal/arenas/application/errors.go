package application

import "errors"

var (
	// ErrArenaNotFound indicates the Arena does not exist or does not belong
	// to the requesting creator. Foreign drafts are deliberately
	// indistinguishable from missing ones: probing someone else's id leaks
	// nothing.
	ErrArenaNotFound = errors.New("application: arena not found")

	// ErrVersionConflict indicates the optimistic version check failed: the
	// Arena changed since the caller read it.
	ErrVersionConflict = errors.New("application: arena version conflict")

	// ErrNoPassAvailable indicates the creator holds no valid Arena Pass, so
	// the publication cannot consume one.
	ErrNoPassAvailable = errors.New("application: no arena pass available")

	// ErrArenaAlreadyConsumed indicates the consumption for this Arena
	// belongs to another account; passes are never transferable.
	ErrArenaAlreadyConsumed = errors.New("application: arena pass was already consumed by another account")

	// ErrSlugConflict indicates the derived public slug is already taken by
	// another Arena.
	ErrSlugConflict = errors.New("application: arena slug is already taken")

	// ErrNotAuthorized indicates the acting account is not allowed to perform
	// moderation actions on Arenas.
	ErrNotAuthorized = errors.New("application: actor is not authorized for arena moderation")

	// ErrInvalidCursor indicates a malformed, forged or version-mismatched
	// feed cursor. Unknown values are never reflected back to callers.
	ErrInvalidCursor = errors.New("application: arena feed cursor is invalid")

	// ErrWeakFeedCursorSecret indicates the configured feed cursor signing
	// secret is shorter than the 256-bit minimum.
	ErrWeakFeedCursorSecret = errors.New("application: arena feed cursor secret must be at least 32 bytes")

	// ErrInvalidFeedFilter indicates a feed filter that can never describe a
	// public Arena (unknown status, draft or removed).
	ErrInvalidFeedFilter = errors.New("application: arena feed filter is invalid")

	// ErrArenaGone indicates the Arena existed and was addressed publicly
	// before, but was removed by moderation. Removed is terminal, so the
	// document endpoint answers 410 instead of 404.
	ErrArenaGone = errors.New("application: arena was removed")
)
