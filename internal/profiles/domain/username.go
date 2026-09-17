package domain

import (
	"strings"
	"unicode/utf8"
)

// Username length bounds mirror the CHECK constraints of the app.profiles
// migration (00005_profiles_schema.sql) so the domain and the database
// reject the same inputs.
const (
	UsernameMinLength = 3
	UsernameMaxLength = 30
)

// Username is an immutable value object for the public handle.
//
// The display form preserves the caller's ASCII casing for presentation;
// the normalized form (ASCII lowercase) is the uniqueness key persisted in
// app.profiles.username_normalized and the only value used for lookups and
// comparisons.
//
// Usernames are deliberately ASCII-only: Unicode letters, confusables and
// bidi controls are rejected at parse time, before any normalization can
// fold them into an existing handle. Visual collisions inside the allowed
// alphabet — for example "l" vs "1", "O" vs "0" or "rn" vs "m" — are a
// documented residual risk: each normalized form is a distinct handle, so
// impersonation is a moderation concern, not a syntax one.
type Username struct {
	display    string
	normalized string
}

// ParseUsername validates, normalizes and constructs a Username.
//
// The raw input is trimmed, required to be valid ASCII, bounded by
// UsernameMinLength/UsernameMaxLength and restricted to letters, digits and
// inner '-'/'_' (never leading or trailing). The normalized form is the
// lowercase ASCII form.
func ParseUsername(raw string) (Username, error) {
	if !utf8.ValidString(raw) {
		return Username{}, ErrInvalidUsernameFormat
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Username{}, ErrEmptyUsername
	}

	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] >= utf8.RuneSelf {
			return Username{}, ErrUsernameNonASCII
		}
	}

	if len(trimmed) < UsernameMinLength {
		return Username{}, ErrUsernameTooShort
	}
	if len(trimmed) > UsernameMaxLength {
		return Username{}, ErrUsernameTooLong
	}

	if !isUsernameAlphanumeric(trimmed[0]) || !isUsernameAlphanumeric(trimmed[len(trimmed)-1]) {
		return Username{}, ErrInvalidUsernameFormat
	}
	for i := 1; i < len(trimmed)-1; i++ {
		if !isUsernameByte(trimmed[i]) {
			return Username{}, ErrInvalidUsernameFormat
		}
	}

	return Username{display: trimmed, normalized: strings.ToLower(trimmed)}, nil
}

func isUsernameAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isUsernameByte(b byte) bool {
	return isUsernameAlphanumeric(b) || b == '-' || b == '_'
}

// String returns the display form of the username.
func (u Username) String() string {
	return u.display
}

// Normalized returns the canonical lowercase form used for uniqueness,
// lookups and equality.
func (u Username) Normalized() string {
	return u.normalized
}

// Equals reports whether two usernames resolve to the same handle. Two
// usernames that differ only in ASCII casing are equal.
func (u Username) Equals(other Username) bool {
	return u.normalized == other.normalized
}

// IsZero reports whether the Username is the uninitialized zero value.
func (u Username) IsZero() bool {
	return u.normalized == ""
}
