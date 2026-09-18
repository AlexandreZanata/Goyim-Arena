package application

import "errors"

var (
	// ErrInvalidMetricsConfig indicates the derive use case could not be
	// built from the given dependencies.
	ErrInvalidMetricsConfig = errors.New("application: transparency metrics configuration is invalid")

	// ErrInvalidExportConfig indicates the export use case could not be
	// built from the given dependencies.
	ErrInvalidExportConfig = errors.New("application: transparency export configuration is invalid")

	// ErrWeakExportCursorSecret indicates the configured export cursor
	// signing secret is shorter than 256 bits.
	ErrWeakExportCursorSecret = errors.New("application: transparency export cursor secret is too short")

	// ErrInvalidCursor indicates a forged, corrupted or version-mismatched
	// cursor was presented; it is never reflected back.
	ErrInvalidCursor = errors.New("application: transparency cursor is invalid")

	// ErrArenaNotFound indicates the Arena is unknown, still a draft or no
	// longer publicly readable (removed), all indistinguishable on purpose.
	ErrArenaNotFound = errors.New("application: publicly readable arena not found")
)
