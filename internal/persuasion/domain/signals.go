package domain

import (
	"sort"
	"time"
)

// AbuseSignal is the advisory vocabulary of attribution manipulation signals
// (P11-T07; THR-PERS-01, MODERATION §3, METRICS §4, SECURITY §6). A signal is
// an observation, never a decision: it carries the counts that crossed a
// versioned threshold, never a score, a severity or an automatic action.
// Nothing here removes content, invalidates an attribution or blocks an
// account — moderation decides, with human review (MODERATION §5, §10).
type SignalKind string

const (
	// SignalReciprocity flags a counterpart that credited the subject's
	// arguments while the subject credited the counterpart's arguments, at
	// least once in each direction: repeated reciprocity is the ring shape
	// THR-PERS-01 describes.
	SignalReciprocity SignalKind = "reciprocity"

	// SignalConcentration flags one account holding a dominant share of the
	// subject's valid attributions in the window (METRICS §4).
	SignalConcentration SignalKind = "concentration"

	// SignalRapidAlternation flags repeated position reversals by one
	// account in the window (METRICS §4): flipping back and forth is how
	// repeated attribution is farmed.
	SignalRapidAlternation SignalKind = "rapid_alternation"
)

// IsValid reports whether the kind is an authorized enum value.
func (k SignalKind) IsValid() bool {
	switch k {
	case SignalReciprocity, SignalConcentration, SignalRapidAlternation:
		return true
	default:
		return false
	}
}

// basisPointTotal is the denominator of an integer share: shares are compared
// in basis points so no signal ever depends on floating point arithmetic.
const basisPointTotal = 10_000

// SignalPolicy is the versioned threshold set of the abuse signal assessment.
// Thresholds are configuration injected at bootstrap, never content
// judgments: the same policy evaluates every subject, and a signal never says
// an opinion is wrong (MODERATION §1). The defaults are deliberately
// conservative — a signal is a reason to look, not proof (MODERATION §10).
type SignalPolicy struct {
	// Version identifies the configuration revision that produced an
	// assessment; it travels with every signal so a reviewed case can be
	// explained later (TRANSPARENCY.md).
	Version string

	// Window bounds the observed facts. Behaviour older than the window
	// never contributes, so signals describe a recent pattern.
	Window time.Duration

	// MinReciprocityEvents is the smallest number of events each direction
	// must hold before mutual attribution counts as a ring. One exchange in
	// each direction is ordinary social behaviour and stays unreported.
	MinReciprocityEvents int64

	// MinConcentrationEvents is the smallest number of valid attributions
	// one account must hold before its share is meaningful: in a young Arena
	// a single participant can hold 100% of two events, which is not abuse.
	MinConcentrationEvents int64

	// ConcentrationShareBasisPoints is the dominant share, in basis points,
	// above which one account concentrates the subject's attributions.
	ConcentrationShareBasisPoints int64

	// MinAlternationChanges is the smallest number of position changes one
	// account must register in the window before reversals are considered.
	MinAlternationChanges int64

	// MinAlternationReversals is the smallest number of direction reversals
	// (a change back to the position held before the previous change) that,
	// together with MinAlternationChanges, marks repeated flipping.
	MinAlternationReversals int64
}

// DefaultSignalPolicy returns the initial MVP policy. Every threshold is a
// documented starting point meant to be revised with real data, and each one
// is paired with a documented false positive in the tests.
func DefaultSignalPolicy() SignalPolicy {
	return SignalPolicy{
		Version:                       "2026-09-attribution-signals-v1",
		Window:                        30 * 24 * time.Hour,
		MinReciprocityEvents:          2,
		MinConcentrationEvents:        4,
		ConcentrationShareBasisPoints: 6_000,
		MinAlternationChanges:         4,
		MinAlternationReversals:       2,
	}
}

// IsValid reports whether the policy is complete and coherent.
func (p SignalPolicy) IsValid() bool {
	if p.Version == "" || p.Window <= 0 {
		return false
	}
	if p.MinReciprocityEvents < 1 {
		return false
	}
	if p.MinConcentrationEvents < 1 {
		return false
	}
	if p.ConcentrationShareBasisPoints < 1 || p.ConcentrationShareBasisPoints > basisPointTotal {
		return false
	}
	if p.MinAlternationChanges < 2 || p.MinAlternationReversals < 1 {
		return false
	}
	if p.MinAlternationReversals >= p.MinAlternationChanges {
		// Reversals are a subset of the changes counted in the window: a
		// policy requiring more reversals than changes could never match.
		return false
	}
	return true
}

// ReciprocityFact is one counterpart that credited the subject's arguments
// while the subject credited the counterpart's arguments in the window. Both
// counts are valid attribution events; the account identity stays internal —
// signals are restricted data (CONSTITUTION §Dados pessoais).
type ReciprocityFact struct {
	Counterpart AttributorID
	// Inbound counts the attributions the counterpart made to the subject's
	// arguments in the window.
	Inbound int64
	// Outbound counts the attributions the subject made to the counterpart's
	// arguments in the window.
	Outbound int64
}

// ConcentrationFact is one account's valid attributions to the subject's
// arguments in the window.
type ConcentrationFact struct {
	Attributor AttributorID
	Events     int64
}

// AlternationFact is one account's position changes in the window, together
// with how many of them were reversals.
type AlternationFact struct {
	Account   AttributorID
	Changes   int64
	Reversals int64
}

// SignalFacts are the observations one assessment reads. They are pure data:
// the adapter aggregates them, the policy judges them, and nothing else is
// needed to explain a signal.
type SignalFacts struct {
	Reciprocity   []ReciprocityFact
	Concentration []ConcentrationFact
	Alternation   []AlternationFact
}

// Validate rejects malformed facts before they can produce a signal: a
// negative count or a missing account would make an assessment unexplainable.
func (f SignalFacts) Validate() error {
	for _, fact := range f.Reciprocity {
		if fact.Counterpart.IsZero() {
			return ErrInvalidSignalFacts
		}
		if fact.Inbound < 0 || fact.Outbound < 0 {
			return ErrInvalidSignalFacts
		}
	}
	for _, fact := range f.Concentration {
		if fact.Attributor.IsZero() || fact.Events < 0 {
			return ErrInvalidSignalFacts
		}
	}
	for _, fact := range f.Alternation {
		if fact.Account.IsZero() || fact.Changes < 0 || fact.Reversals < 0 || fact.Reversals > fact.Changes {
			return ErrInvalidSignalFacts
		}
	}
	return nil
}

// Signal is one observation that crossed a threshold. It carries the subject,
// the counterpart involved (when the kind names one) and the counts that
// crossed the threshold — never a score, a weight or a recommended action.
// "Penalização de peso algorítmico" (THR-PERS-01) is deliberately absent from
// the MVP: no metric is silently reweighted.
type Signal struct {
	Kind SignalKind
	// Counterpart is the account whose behaviour crossed the threshold. It is
	// an internal identifier: signals are restricted (never public, never
	// exported; CONSTITUTION §Dados pessoais).
	Counterpart AttributorID
	// Window is the assessed period, carried so a reviewer reads the counts
	// with the window that produced them.
	Window time.Duration
	// MutualEvents is the total mutual attribution events (both directions)
	// of a reciprocity signal; zero for the other kinds.
	MutualEvents int64
	// DominantEvents is the concentrated account's valid attributions of a
	// concentration signal; zero for the other kinds.
	DominantEvents int64
	// ShareBasisPoints is the dominant account's share, in basis points, of
	// the subject's valid attributions in the window; zero for the other
	// kinds.
	ShareBasisPoints int64
	// Changes and Reversals are the position changes and the reversals of a
	// rapid alternation signal; zero for the other kinds.
	Changes   int64
	Reversals int64
}

// Validate rejects a signal that does not carry the counts its kind requires,
// so an unexplainable observation can never be served or stored.
func (s Signal) Validate() error {
	if !s.Kind.IsValid() || s.Counterpart.IsZero() || s.Window <= 0 {
		return ErrInvalidSignal
	}
	switch s.Kind {
	case SignalReciprocity:
		if s.MutualEvents < 1 || s.DominantEvents != 0 || s.ShareBasisPoints != 0 || s.Changes != 0 || s.Reversals != 0 {
			return ErrInvalidSignal
		}
	case SignalConcentration:
		if s.DominantEvents < 1 || s.ShareBasisPoints < 1 || s.ShareBasisPoints > basisPointTotal || s.MutualEvents != 0 || s.Changes != 0 || s.Reversals != 0 {
			return ErrInvalidSignal
		}
	case SignalRapidAlternation:
		if s.Changes < 1 || s.Reversals < 1 || s.Reversals > s.Changes || s.MutualEvents != 0 || s.DominantEvents != 0 || s.ShareBasisPoints != 0 {
			return ErrInvalidSignal
		}
	}
	return nil
}

// SignalAssessment is the outcome of one assessment: the signals that crossed
// the thresholds, the instant of the reading and the policy revision used.
// An assessment is advisory and read-only: producing it writes nothing and
// changes no metric, no validity and no access.
type SignalAssessment struct {
	Subject AuthorID
	// Signals are ordered deterministically by kind and counterpart so the
	// same facts always produce the same document.
	Signals       []Signal
	PolicyVersion string
	Window        time.Duration
	AssessedAt    time.Time
}

// Has reports whether a signal of the given kind was produced.
func (a SignalAssessment) Has(kind SignalKind) bool {
	for _, signal := range a.Signals {
		if signal.Kind == kind {
			return true
		}
	}
	return false
}

// Validate rejects an incoherent assessment: a missing subject or instant, a
// missing policy revision, a signal outside the assessed window or the same
// observation reported twice.
func (a SignalAssessment) Validate() error {
	if a.Subject.IsZero() || a.AssessedAt.IsZero() || a.PolicyVersion == "" || a.Window <= 0 {
		return ErrInvalidSignalAssessment
	}
	seen := make(map[string]bool, len(a.Signals))
	for _, signal := range a.Signals {
		if err := signal.Validate(); err != nil {
			return err
		}
		if signal.Window != a.Window {
			return ErrInvalidSignalAssessment
		}
		key := string(signal.Kind) + ":" + signal.Counterpart.String()
		if seen[key] {
			return ErrInvalidSignalAssessment
		}
		seen[key] = true
	}
	return nil
}

// Assess applies the policy to the observed facts and returns the signals that
// crossed the thresholds. It is pure: the same facts and policy produce the
// same signals, and no fact outside the thresholds becomes a signal — the
// documented false positives stay unreported by construction.
func (p SignalPolicy) Assess(subject AuthorID, facts SignalFacts) ([]Signal, error) {
	if !p.IsValid() {
		return nil, ErrInvalidSignalPolicy
	}
	if subject.IsZero() {
		return nil, ErrEmptyAuthorID
	}
	if err := facts.Validate(); err != nil {
		return nil, err
	}

	signals := make([]Signal, 0, len(facts.Reciprocity)+len(facts.Concentration)+len(facts.Alternation))

	for _, fact := range facts.Reciprocity {
		// Mutual attribution counts as a ring only when both directions hold
		// at least the configured number of events: a single exchange each way
		// is ordinary social behaviour (documented false positive).
		if fact.Inbound < p.MinReciprocityEvents || fact.Outbound < p.MinReciprocityEvents {
			continue
		}
		signals = append(signals, Signal{
			Kind:         SignalReciprocity,
			Counterpart:  fact.Counterpart,
			Window:       p.Window,
			MutualEvents: fact.Inbound + fact.Outbound,
		})
	}

	var total int64
	for _, fact := range facts.Concentration {
		total += fact.Events
	}
	for _, fact := range facts.Concentration {
		if fact.Events < p.MinConcentrationEvents || total == 0 {
			continue
		}
		share := fact.Events * basisPointTotal / total
		if share < p.ConcentrationShareBasisPoints {
			continue
		}
		signals = append(signals, Signal{
			Kind:             SignalConcentration,
			Counterpart:      fact.Attributor,
			Window:           p.Window,
			DominantEvents:   fact.Events,
			ShareBasisPoints: share,
		})
	}

	for _, fact := range facts.Alternation {
		// Repeated flipping needs both enough activity and enough reversals:
		// reading one Arena in several topics and changing position in
		// different Arenas is ordinary engagement (documented false positive).
		if fact.Changes < p.MinAlternationChanges || fact.Reversals < p.MinAlternationReversals {
			continue
		}
		signals = append(signals, Signal{
			Kind:        SignalRapidAlternation,
			Counterpart: fact.Account,
			Window:      p.Window,
			Changes:     fact.Changes,
			Reversals:   fact.Reversals,
		})
	}

	sort.SliceStable(signals, func(i, j int) bool {
		if signals[i].Kind != signals[j].Kind {
			return signals[i].Kind < signals[j].Kind
		}
		return signals[i].Counterpart.String() < signals[j].Counterpart.String()
	})
	return signals, nil
}
