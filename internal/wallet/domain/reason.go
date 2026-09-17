package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxReasonLength bounds administrative justifications; 500 characters fit
// any real operator note while keeping stored reasons bounded.
const maxReasonLength = 500

// Reason is an immutable, non-empty justification of an administrative
// adjustment. It is human-readable prose (accented text is fine) but never
// transport markup: control characters and bidirectional overrides are
// rejected so the value stays safe to display and to log.
type Reason struct {
	value string
}

// ParseReason validates and trims an administrative reason.
func ParseReason(raw string) (Reason, error) {
	if !utf8.ValidString(raw) {
		return Reason{}, ErrInvalidReason
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Reason{}, ErrEmptyReason
	}
	if utf8.RuneCountInString(trimmed) > maxReasonLength {
		return Reason{}, ErrReasonTooLong
	}

	for _, r := range trimmed {
		if unicode.IsControl(r) || isBidiOverride(r) {
			return Reason{}, ErrInvalidReason
		}
	}

	return Reason{value: trimmed}, nil
}

// isBidiOverride reports whether the rune is a bidirectional control that
// could visually reorder a displayed reason.
func isBidiOverride(r rune) bool {
	switch r {
	case '\u200E', '\u200F', '\u061C',
		'\u202A', '\u202B', '\u202C', '\u202D', '\u202E',
		'\u2066', '\u2067', '\u2068', '\u2069':
		return true
	default:
		return false
	}
}

// String returns the stored reason.
func (r Reason) String() string {
	return r.value
}

// IsZero reports whether the Reason is the uninitialized zero value.
func (r Reason) IsZero() bool {
	return r.value == ""
}

// Equals reports whether two reasons are identical.
func (r Reason) Equals(other Reason) bool {
	return r.value == other.value
}
