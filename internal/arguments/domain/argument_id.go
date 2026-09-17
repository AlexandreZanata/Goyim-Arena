package domain

import "strings"

// maxArgumentIDLength bounds opaque identifiers; a UUID stays well below it.
const maxArgumentIDLength = 64

// ArgumentID is an immutable, opaque identifier of an argument. The
// arguments domain never interprets it beyond identity.
type ArgumentID struct {
	value string
}

// ParseArgumentID validates and trims an argument identifier. It accepts
// printable ASCII only so stored identifiers stay stable and safe to log.
func ParseArgumentID(raw string) (ArgumentID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ArgumentID{}, ErrEmptyArgumentID
	}
	if len(trimmed) > maxArgumentIDLength {
		return ArgumentID{}, ErrInvalidArgumentID
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return ArgumentID{}, ErrInvalidArgumentID
		}
	}
	return ArgumentID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id ArgumentID) String() string {
	return id.value
}

// IsZero reports whether the ArgumentID is the uninitialized zero value.
func (id ArgumentID) IsZero() bool {
	return id.value == ""
}

// Equals reports whether two argument identifiers are identical.
func (id ArgumentID) Equals(other ArgumentID) bool {
	return id.value == other.value
}
