package domain

// AggregatePolicy is the versioned privacy policy of the public position
// aggregates. Below MinParticipants every count is withheld, so a tiny
// sample can never reveal an individual position (docs/PRIVACY.md,
// docs/BUSINESS_RULES.md §7). The values are configuration injected at
// bootstrap, never content judgments.
type AggregatePolicy struct {
	// Version identifies the configuration revision the threshold came
	// from.
	Version string
	// MinParticipants is the smallest eligible population whose
	// distribution may be published.
	MinParticipants int64
}

// DefaultAggregatePolicy returns the initial privacy threshold of the MVP:
// provisional, configurable through the injected policy.
func DefaultAggregatePolicy() AggregatePolicy {
	return AggregatePolicy{
		Version:         "2026-09",
		MinParticipants: 10,
	}
}

// IsValid reports whether the policy is internally coherent.
func (p AggregatePolicy) IsValid() bool {
	return p.Version != "" && p.MinParticipants >= 1
}

// Suppresses reports whether a population of the given size must have every
// count withheld.
func (p AggregatePolicy) Suppresses(total int64) bool {
	return total < p.MinParticipants
}
