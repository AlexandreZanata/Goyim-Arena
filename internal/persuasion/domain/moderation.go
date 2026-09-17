package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxReasonLength bounds moderation justifications: 500 characters fit any
// real operator note while keeping stored reasons bounded (the same bound
// the wallet administrative adjustments and the Arena moderation use).
const maxReasonLength = 500

// AttributionID is an immutable, opaque identifier of one recorded
// attribution. The persuasion domain never interprets it beyond identity.
type AttributionID struct {
	value string
}

// ParseAttributionID validates and trims an attribution identifier.
func ParseAttributionID(raw string) (AttributionID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyAttributionID)
	if err != nil {
		return AttributionID{}, err
	}
	return AttributionID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id AttributionID) String() string { return id.value }

// IsZero reports whether the AttributionID is the uninitialized zero value.
func (id AttributionID) IsZero() bool { return id.value == "" }

// Equals reports whether two attribution identifiers are identical.
func (id AttributionID) Equals(other AttributionID) bool { return id.value == other.value }

// ModeratorID is the acting moderator account of a moderation decision. The
// persuasion module never decides roles: the authorizer port answers whether
// this account may moderate.
type ModeratorID struct {
	value string
}

// ParseModeratorID validates and trims a moderator identifier.
func ParseModeratorID(raw string) (ModeratorID, error) {
	trimmed, err := parseIdentifier(raw, ErrEmptyModeratorID)
	if err != nil {
		return ModeratorID{}, err
	}
	return ModeratorID{value: trimmed}, nil
}

// String returns the stored identifier.
func (id ModeratorID) String() string { return id.value }

// IsZero reports whether the ModeratorID is the uninitialized zero value.
func (id ModeratorID) IsZero() bool { return id.value == "" }

// Equals reports whether two moderator identifiers are identical.
func (id ModeratorID) Equals(other ModeratorID) bool { return id.value == other.value }

// AttributionStatus is the closed validity vocabulary of a recorded
// attribution. Invalid attributions are kept as historical facts and are
// excluded from valid metrics; they are never deleted (BR §5.1, §6).
type AttributionStatus string

const (
	// AttributionStatusValid counts towards valid reputation metrics.
	AttributionStatusValid AttributionStatus = "valid"

	// AttributionStatusInvalid was invalidated by moderation for fraud or
	// collusion and no longer counts, while the record is preserved.
	AttributionStatusInvalid AttributionStatus = "invalid"
)

// IsValid reports whether the status is an authorized enum value.
func (s AttributionStatus) IsValid() bool {
	switch s {
	case AttributionStatusValid, AttributionStatusInvalid:
		return true
	default:
		return false
	}
}

// String returns the stored status.
func (s AttributionStatus) String() string { return string(s) }

// ParseAttributionStatus validates one stored validity value.
func ParseAttributionStatus(raw string) (AttributionStatus, error) {
	status := AttributionStatus(raw)
	if !status.IsValid() {
		return "", ErrInvalidAttributionStatus
	}
	return status, nil
}

// ModerationAction is the closed vocabulary of attribution moderation
// decisions.
type ModerationAction string

const (
	// ModerationActionInvalidate marks an attribution as fraudulent or
	// collusive without deleting it.
	ModerationActionInvalidate ModerationAction = "invalidate"

	// ModerationActionRestore reverses an invalidation.
	ModerationActionRestore ModerationAction = "restore"
)

// IsValid reports whether the action is an authorized enum value.
func (a ModerationAction) IsValid() bool {
	switch a {
	case ModerationActionInvalidate, ModerationActionRestore:
		return true
	default:
		return false
	}
}

// String returns the stored action.
func (a ModerationAction) String() string { return string(a) }

// TargetStatus returns the validity the action moves the attribution to, or
// the empty status for an unrecognized action.
func (a ModerationAction) TargetStatus() AttributionStatus {
	switch a {
	case ModerationActionInvalidate:
		return AttributionStatusInvalid
	case ModerationActionRestore:
		return AttributionStatusValid
	default:
		return ""
	}
}

// Reason is an immutable, non-empty justification of a moderation decision.
// It is human-readable prose (accented text is fine) but never transport
// markup: control characters and bidirectional overrides are rejected so the
// value stays safe to display and to log.
type Reason struct {
	value string
}

// ParseReason validates and trims a moderation reason.
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

// String returns the stored reason.
func (r Reason) String() string { return r.value }

// IsZero reports whether the Reason is the uninitialized zero value.
func (r Reason) IsZero() bool { return r.value == "" }

// Equals reports whether two reasons are identical.
func (r Reason) Equals(other Reason) bool { return r.value == other.value }

// isBidiOverride reports whether the rune is a bidirectional control that
// could visually reorder a stored justification.
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

// ModerationDecision is the recorded administrative decision over one
// attribution: the action, the acting moderator, the mandatory reason and
// the instant. The decision is preserved on the retained row, so no
// invalidation exists without its record (REQ-PERS-08).
type ModerationDecision struct {
	Action    ModerationAction
	Actor     ModeratorID
	Reason    Reason
	DecidedAt time.Time
}

// IsZero reports whether no decision is recorded.
func (d ModerationDecision) IsZero() bool { return d.Action == "" }

// TargetStatus returns the validity this decision moves the attribution to.
func (d ModerationDecision) TargetStatus() AttributionStatus { return d.Action.TargetStatus() }

// Validate checks that the decision is complete: a known action, an acting
// moderator, a non-empty reason and a decision instant.
func (d ModerationDecision) Validate() error {
	if !d.Action.IsValid() {
		return ErrInvalidModerationAction
	}
	if d.Actor.IsZero() {
		return ErrEmptyModeratorID
	}
	if d.Reason.IsZero() {
		return ErrEmptyReason
	}
	if d.DecidedAt.IsZero() {
		return ErrInvalidInstant
	}
	return nil
}

// Attribution is one recorded influence attribution: one position change
// credits one argument. It carries the current validity and the latest
// moderation decision; the links, the private attributor and the creation
// instant never change.
type Attribution struct {
	ID           AttributionID
	ChangeID     ChangeID
	ArgumentID   ArgumentID
	AttributorID AttributorID
	Status       AttributionStatus
	CreatedAt    time.Time
	Decision     ModerationDecision
}

// IsValid reports whether the attribution currently counts towards valid
// metrics.
func (a Attribution) IsValid() bool { return a.Status == AttributionStatusValid }

// HasDecision reports whether a moderation decision is recorded on the
// attribution.
func (a Attribution) HasDecision() bool { return !a.Decision.IsZero() }

// DecisionOf returns the recorded decision, if any.
func (a Attribution) DecisionOf() (ModerationDecision, bool) { return a.Decision, a.HasDecision() }

// Invalidate records an invalidation decision and moves the attribution to
// invalid. It never deletes or rewrites the attribution itself.
func (a Attribution) Invalidate(decision ModerationDecision) (Attribution, error) {
	if decision.Action != ModerationActionInvalidate {
		return Attribution{}, ErrInvalidModerationAction
	}
	if err := decision.Validate(); err != nil {
		return Attribution{}, err
	}
	if !a.IsValid() {
		return Attribution{}, ErrAttributionNotModeratable
	}
	a.Status = AttributionStatusInvalid
	a.Decision = decision
	return a, nil
}

// Restore records a restoration decision and moves the invalidation back to
// valid, keeping the decision record (the restoration replaces it, never
// erases it).
func (a Attribution) Restore(decision ModerationDecision) (Attribution, error) {
	if decision.Action != ModerationActionRestore {
		return Attribution{}, ErrInvalidModerationAction
	}
	if err := decision.Validate(); err != nil {
		return Attribution{}, err
	}
	if a.IsValid() {
		return Attribution{}, ErrAttributionNotModeratable
	}
	a.Status = AttributionStatusValid
	a.Decision = decision
	return a, nil
}
