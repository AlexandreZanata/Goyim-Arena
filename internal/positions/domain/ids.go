package domain

import "strings"

// maxIdentifierLength bounds opaque identifiers; a UUID stays well below it.
const maxIdentifierLength = 64

// ArenaID is an immutable, opaque identifier of the Arena a position refers
// to. The positions domain never interprets it beyond identity: the Arena
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
	if len(trimmed) > maxIdentifierLength {
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

// AccountID is an immutable, opaque identifier of the account that holds a
// position. The positions domain never interprets it beyond identity.
type AccountID struct {
	value string
}

// ParseAccountID validates and trims an account identifier. It accepts
// printable ASCII only so stored identifiers stay stable and safe to log.
func ParseAccountID(raw string) (AccountID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return AccountID{}, ErrEmptyAccountID
	}
	if len(trimmed) > maxIdentifierLength {
		return AccountID{}, ErrInvalidAccountID
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return AccountID{}, ErrInvalidAccountID
		}
	}
	return AccountID{value: trimmed}, nil
}

// String returns the stored account identifier.
func (id AccountID) String() string {
	return id.value
}

// IsZero reports whether the AccountID is the uninitialized zero value.
func (id AccountID) IsZero() bool {
	return id.value == ""
}

// Equals reports whether two account identifiers are identical.
func (id AccountID) Equals(other AccountID) bool {
	return id.value == other.value
}
