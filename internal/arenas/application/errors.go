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
)
