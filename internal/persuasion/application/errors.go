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
)
