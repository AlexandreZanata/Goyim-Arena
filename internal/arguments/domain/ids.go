package domain

import "strings"

// maxIdentifierLength bounds opaque identifiers; a UUID stays well below it.
const maxIdentifierLength = 64

// ArenaID is an immutable, opaque identifier of the Arena an argument
// belongs to. The arguments domain never interprets it beyond identity.
type ArenaID struct {
	value string
}

// ParseArenaID validates and trims an Arena identifier. It accepts printable
// ASCII only so stored identifiers stay stable and safe to log.
func ParseArenaID(raw string) (ArenaID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyArenaID, ErrInvalidArenaID)
	if err != nil {
		return ArenaID{}, err
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

// AccountID is an immutable, opaque identifier of the account that authored
// an argument. The arguments domain never interprets it beyond identity.
type AccountID struct {
	value string
}

// ParseAccountID validates and trims an account identifier. It accepts
// printable ASCII only so stored identifiers stay stable and safe to log.
func ParseAccountID(raw string) (AccountID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyAccountID, ErrInvalidAccountID)
	if err != nil {
		return AccountID{}, err
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

// parseIdentifier applies the shared structural rules of opaque identifiers.
func parseIdentifier(raw string, emptyErr, invalidErr DomainError) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", emptyErr
	}
	if len(trimmed) > maxIdentifierLength {
		return "", invalidErr
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return "", invalidErr
		}
	}
	return trimmed, nil
}
