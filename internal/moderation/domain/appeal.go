package domain

import "time"

// Outcome is the terminal result of an appeal review. It mirrors the
// decided vocabulary of app.moderation_appeals (migration 00022): upheld
// keeps the sanction, modified adjusts its reading without rewriting the
// original action, reversed restores the sanctioned projection. Every
// outcome carries the reviewer's reason; the original action row is never
// edited or deleted.
type Outcome string

const (
	// OutcomeUpheld keeps the sanctioned state.
	OutcomeUpheld Outcome = "upheld"
	// OutcomeModified re-reads the sanction without rewriting history.
	OutcomeModified Outcome = "modified"
	// OutcomeReversed restores the sanctioned projection from the original
	// action record.
	OutcomeReversed Outcome = "reversed"
)

// AllOutcomes returns the closed vocabulary in canonical order.
func AllOutcomes() []Outcome {
	return []Outcome{OutcomeUpheld, OutcomeModified, OutcomeReversed}
}

// ParseOutcome validates a requested outcome against the exact vocabulary.
func ParseOutcome(raw string) (Outcome, error) {
	outcome := Outcome(raw)
	if !outcome.IsValid() {
		return "", ErrInvalidOutcome
	}
	return outcome, nil
}

// IsValid reports whether the outcome is an authorized enum value.
func (o Outcome) IsValid() bool {
	switch o {
	case OutcomeUpheld, OutcomeModified, OutcomeReversed:
		return true
	default:
		return false
	}
}

// String returns the stored outcome value.
func (o Outcome) String() string {
	return string(o)
}

// RestoresProjection reports whether the outcome moves a public projection
// back. Only reversals restore; upheld and modified leave projections
// untouched while still recording the review.
func (o Outcome) RestoresProjection() bool {
	return o == OutcomeReversed
}

// AppealWindow bounds when a sanction may be contested, counted from the
// action instant. It is a policy constant, not configuration: without a
// deadline sanctions could never settle, and with a per-case deadline
// reviewers could not plan. Late appeals deny with a distinct error.
const AppealWindow = 30 * 24 * time.Hour

// Appealable reports whether the action admits an appeal. A no_action
// decision sanctions nobody, so there is no affected owner to appeal it.
func Appealable(action Action) bool {
	if !action.IsValid() {
		return false
	}
	return action != ActionNoAction
}

// AppealExpired reports whether the action instant is too old to contest
// at the instant.
func AppealExpired(actionAt, now time.Time) bool {
	return now.Sub(actionAt.UTC()) > AppealWindow
}
