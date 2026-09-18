package domain

import (
	"strings"
	"time"
)

// MaxRuleLength bounds the applied rule reference. It mirrors the CHECK
// constraint of app.moderation_actions (migration 00022).
const MaxRuleLength = 200

// MaxJustificationLength bounds the internal decision justification. It
// mirrors the CHECK constraint of app.moderation_actions (migration 00022).
// Justifications are restricted evidence: stored, never logged, never
// projected publicly.
const MaxJustificationLength = 2000

// ParseRule validates the applied rule reference: a non-blank stable
// reference (for example MOD-3:spam) chosen by the moderator. The rule is
// never inferred from content: the measure chosen stays the moderator's
// explicit decision.
func ParseRule(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > MaxRuleLength {
		return "", ErrInvalidRule
	}
	return trimmed, nil
}

// ParseJustification validates the internal decision justification:
// non-blank text within the bound.
func ParseJustification(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > MaxJustificationLength {
		return "", ErrInvalidJustification
	}
	return trimmed, nil
}

// ClaimLease bounds how long one moderator owns a review. It matches the
// lease written by the claim path: a live lease serializes concurrent
// claims, an expired lease may be reclaimed by another moderator.
const ClaimLease = 15 * time.Minute

// DecisionExpiry validates the optional expiry of a sanction against the
// action and the decision instant: suspensions and interaction limits
// require a future expiry, every other action forbids one. It mirrors the
// CHECK constraint of app.moderation_actions (migration 00022) so a value
// accepted here is a value the database accepts.
func DecisionExpiry(action Action, expiresAt *time.Time, decidedAt time.Time) (*time.Time, error) {
	if !action.IsValid() {
		return nil, ErrInvalidAction
	}
	needsExpiry := action == ActionSuspension || action == ActionInteractionLimit
	if !needsExpiry {
		if expiresAt != nil {
			return nil, ErrInvalidExpiry
		}
		return nil, nil
	}
	if expiresAt == nil {
		return nil, ErrInvalidExpiry
	}
	instant := expiresAt.UTC()
	if !instant.After(decidedAt.UTC()) {
		return nil, ErrInvalidExpiry
	}
	return &instant, nil
}
