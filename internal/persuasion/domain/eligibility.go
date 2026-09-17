package domain

import (
	"strings"
	"time"
)

// maxIdentifierLength bounds opaque identifiers; a UUID stays well below it.
const maxIdentifierLength = 64

// ChangeID is an immutable, opaque identifier of the position change that
// credits arguments. The persuasion domain never interprets it beyond
// identity.
type ChangeID struct {
	value string
}

// ParseChangeID validates and trims a position change identifier.
func ParseChangeID(raw string) (ChangeID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyChangeID)
	if err != nil {
		return ChangeID{}, err
	}
	return ChangeID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id ChangeID) String() string { return id.value }

// IsZero reports whether the ChangeID is the uninitialized zero value.
func (id ChangeID) IsZero() bool { return id.value == "" }

// Equals reports whether two change identifiers are identical.
func (id ChangeID) Equals(other ChangeID) bool { return id.value == other.value }

// ArgumentID is an immutable, opaque identifier of a credited argument.
type ArgumentID struct {
	value string
}

// ParseArgumentID validates and trims an argument identifier.
func ParseArgumentID(raw string) (ArgumentID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyArgumentID)
	if err != nil {
		return ArgumentID{}, err
	}
	return ArgumentID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id ArgumentID) String() string { return id.value }

// IsZero reports whether the ArgumentID is the uninitialized zero value.
func (id ArgumentID) IsZero() bool { return id.value == "" }

// Equals reports whether two argument identifiers are identical.
func (id ArgumentID) Equals(other ArgumentID) bool { return id.value == other.value }

// ArenaID is an immutable, opaque identifier of the Arena shared by the
// change and its credited arguments.
type ArenaID struct {
	value string
}

// ParseArenaID validates and trims an Arena identifier.
func ParseArenaID(raw string) (ArenaID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyArenaID)
	if err != nil {
		return ArenaID{}, err
	}
	return ArenaID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id ArenaID) String() string { return id.value }

// IsZero reports whether the ArenaID is the uninitialized zero value.
func (id ArenaID) IsZero() bool { return id.value == "" }

// Equals reports whether two Arena identifiers are identical.
func (id ArenaID) Equals(other ArenaID) bool { return id.value == other.value }

// AttributorID is the private account that made the position change.
type AttributorID struct {
	value string
}

// ParseAttributorID validates and trims an attributor identifier.
func ParseAttributorID(raw string) (AttributorID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyAttributorID)
	if err != nil {
		return AttributorID{}, err
	}
	return AttributorID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id AttributorID) String() string { return id.value }

// IsZero reports whether the AttributorID is the uninitialized zero value.
func (id AttributorID) IsZero() bool { return id.value == "" }

// Equals reports whether two attributor identifiers are identical.
func (id AttributorID) Equals(other AttributorID) bool { return id.value == other.value }

// AuthorID is the public account that authored an argument.
type AuthorID struct {
	value string
}

// ParseAuthorID validates and trims an argument author identifier.
func ParseAuthorID(raw string) (AuthorID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyAuthorID)
	if err != nil {
		return AuthorID{}, err
	}
	return AuthorID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id AuthorID) String() string { return id.value }

// IsZero reports whether the AuthorID is the uninitialized zero value.
func (id AuthorID) IsZero() bool { return id.value == "" }

// Equals reports whether two author identifiers are identical.
func (id AuthorID) Equals(other AuthorID) bool { return id.value == other.value }

// ArgumentStatus is the closed visibility vocabulary of an argument at
// selection time.
type ArgumentStatus string

const (
	// ArgumentStatusPublished is publicly available and therefore eligible.
	ArgumentStatusPublished ArgumentStatus = "published"

	// ArgumentStatusWithdrawn was retracted by its author and is no longer
	// eligible.
	ArgumentStatusWithdrawn ArgumentStatus = "withdrawn"

	// ArgumentStatusRemoved was removed by moderation and is no longer
	// eligible.
	ArgumentStatusRemoved ArgumentStatus = "removed"
)

// IsValid reports whether the status is an authorized enum value.
func (s ArgumentStatus) IsValid() bool {
	switch s {
	case ArgumentStatusPublished, ArgumentStatusWithdrawn, ArgumentStatusRemoved:
		return true
	default:
		return false
	}
}

// String returns the stored status.
func (s ArgumentStatus) String() string { return string(s) }

// Change is the position change that credits arguments: its identity, the
// Arena it happened in, the private attributor and the instant.
type Change struct {
	ID           ChangeID
	ArenaID      ArenaID
	AttributorID AttributorID
	ChangedAt    time.Time
}

// Candidate is one argument proposed for attribution, as read at selection
// time. Relation is deliberately absent: the declared relation never
// restricts eligibility (BUSINESS_RULES §5).
type Candidate struct {
	ID        ArgumentID
	ArenaID   ArenaID
	AuthorID  AuthorID
	CreatedAt time.Time
	Status    ArgumentStatus
}

// EligibilityPolicy is the versioned rule set of attribution eligibility.
// The values are configuration injected at bootstrap, never content
// judgments.
type EligibilityPolicy struct {
	// Version identifies the configuration revision the limit came from.
	Version string
	// MaxAttributions is the largest selection one change may credit.
	MaxAttributions int
}

// DefaultEligibilityPolicy returns the initial policy of the MVP: at most
// three arguments per change (BUSINESS_RULES §5).
func DefaultEligibilityPolicy() EligibilityPolicy {
	return EligibilityPolicy{
		Version:         "2026-09",
		MaxAttributions: 3,
	}
}

// IsValid reports whether the policy is internally coherent.
func (p EligibilityPolicy) IsValid() bool {
	return p.Version != "" && p.MaxAttributions >= 1
}

// Validate checks one selection against every eligibility rule and returns
// the first violation. An empty selection is valid: skipping attribution
// never requires a fake entry (P11-T03).
func (p EligibilityPolicy) Validate(change Change, candidates []Candidate) error {
	if !p.IsValid() {
		return ErrInvalidPolicy
	}
	if change.ID.IsZero() {
		return ErrEmptyChangeID
	}
	if change.ArenaID.IsZero() {
		return ErrEmptyArenaID
	}
	if change.AttributorID.IsZero() {
		return ErrEmptyAttributorID
	}
	if change.ChangedAt.IsZero() {
		return ErrInvalidInstant
	}
	if len(candidates) > p.MaxAttributions {
		return ErrTooManyAttributions
	}

	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID.IsZero() {
			return ErrEmptyArgumentID
		}
		if candidate.ArenaID.IsZero() {
			return ErrEmptyArenaID
		}
		if candidate.AuthorID.IsZero() {
			return ErrEmptyAuthorID
		}
		if candidate.CreatedAt.IsZero() {
			return ErrInvalidInstant
		}
		if !candidate.Status.IsValid() {
			return ErrInvalidStatus
		}
		if seen[candidate.ID.String()] {
			return ErrDuplicateAttribution
		}
		seen[candidate.ID.String()] = true

		if !candidate.ArenaID.Equals(change.ArenaID) {
			return ErrCrossArenaArgument
		}
		if candidate.AuthorID.String() == change.AttributorID.String() {
			return ErrSelfAttribution
		}
		if !candidate.CreatedAt.Before(change.ChangedAt) {
			return ErrArgumentNotBeforeChange
		}
		if candidate.Status != ArgumentStatusPublished {
			return ErrArgumentNotEligible
		}
	}
	return nil
}

// parseIdentifier applies the shared structural rules of opaque identifiers.
func parseIdentifier(raw string, emptyErr DomainError) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", emptyErr
	}
	if len(trimmed) > maxIdentifierLength {
		return "", ErrInvalidIdentifier
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] < 0x21 || trimmed[i] > 0x7e {
			return "", ErrInvalidIdentifier
		}
	}
	return trimmed, nil
}
