package domain

import "strings"

// maxArenaIDLength bounds arena identifiers; a UUID stays well below it.
const maxArenaIDLength = 64

// ArenaID is an immutable, opaque identifier of the Arena that consumed a
// pass. The billing domain never interprets it beyond identity: the Arena
// entity belongs to the arenas module.
type ArenaID struct {
	value string
}

// ParseArenaID validates and trims an Arena identifier. It accepts printable
// ASCII only so stored identifiers stay stable and safe to log.
func ParseArenaID(raw string) (ArenaID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ArenaID{}, ErrEmptyArenaID
	}
	if len(trimmed) > maxArenaIDLength {
		return ArenaID{}, ErrInvalidArenaID
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return ArenaID{}, ErrInvalidArenaID
		}
	}
	return ArenaID{value: trimmed}, nil
}

// String returns the stored Arena identifier.
func (id ArenaID) String() string {
	return id.value
}

// IsZero reports whether the ArenaID is the uninitialized zero value.
func (id ArenaID) IsZero() bool {
	return id.value == ""
}

// Equals reports whether two Arena identifiers are identical.
func (id ArenaID) Equals(other ArenaID) bool {
	return id.value == other.value
}
