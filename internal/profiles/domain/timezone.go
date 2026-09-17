package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// maxTimezoneLength bounds stored IANA names; the longest real zone names
// stay far below this and the cap keeps hostile input cheap to reject.
const maxTimezoneLength = 64

// Timezone is an immutable, optional value object for the IANA timezone
// preference. The zero value means "not informed": the product falls back to
// UTC instead of guessing, and clearing the preference is expressed by
// parsing an empty string.
//
// Timezone is independent from Locale: it never changes the interface
// language, and neither preference touches content_language, which belongs
// to Arena content (I18N_STANDARD.md §1).
type Timezone struct {
	name string
}

// ParseTimezone validates an optional IANA timezone name. An empty (or
// whitespace-only) value yields the unset zero Timezone with no error;
// non-empty values must be printable ASCII, bounded and resolvable in the
// timezone database. "Local" is rejected because it is a process-dependent
// alias, not an IANA zone.
func ParseTimezone(raw string) (Timezone, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Timezone{}, nil
	}

	if !utf8.ValidString(trimmed) || len(trimmed) > maxTimezoneLength {
		return Timezone{}, ErrInvalidTimezone
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return Timezone{}, ErrInvalidTimezone
		}
	}
	if trimmed == "Local" {
		return Timezone{}, ErrInvalidTimezone
	}

	if _, err := time.LoadLocation(trimmed); err != nil {
		return Timezone{}, ErrInvalidTimezone
	}

	return Timezone{name: trimmed}, nil
}

// String returns the stored timezone name, or an empty string when unset.
func (t Timezone) String() string {
	return t.name
}

// IsZero reports whether the timezone preference is unset.
func (t Timezone) IsZero() bool {
	return t.name == ""
}

// Equals reports whether two timezones are the same stored name. The zero
// value only equals another unset timezone.
func (t Timezone) Equals(other Timezone) bool {
	return t.name == other.name
}
