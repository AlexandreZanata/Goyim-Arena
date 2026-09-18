package domain

import (
	"strings"
	"time"
)

// RetentionClass is the closed vocabulary of the data classes the
// executable retention policy governs (P14-T07; docs/PRIVACY.md §1/§5). The
// values are the same strings the retention ledger stores, so a recorded run
// and the policy that produced it can never drift apart.
type RetentionClass string

const (
	// RetentionClassTokens covers the single-use authentication token hashes
	// (email verification and password recovery). They exist only to be
	// consumed once, so after their terminal instant they serve no purpose
	// and are purged.
	RetentionClassTokens RetentionClass = "tokens"
	// RetentionClassSessions covers the server-side session store. A
	// revoked or expired session is dead; the row is purged so the token
	// hash and the client references stop existing.
	RetentionClassSessions RetentionClass = "sessions"
	// RetentionClassReferentialLogs covers the append-only administrative
	// trail: the reference log of what the platform did, to which target and
	// under which stable reason code (P14-T01). It is retained as evidence
	// and is never purged.
	RetentionClassReferentialLogs RetentionClass = "referential_logs"
	// RetentionClassExports covers personal data export documents. The
	// record is retained (migration 00026 forbids deleting it) but the
	// document bytes and the download capability are purged once the link
	// expired.
	RetentionClassExports RetentionClass = "exports"
	// RetentionClassAbuseSignals covers the restricted prevention
	// references attached to a session: the client IP address and the user
	// agent. They are personal data of limited usefulness, so they are
	// anonymized long before the session row itself is purged.
	RetentionClassAbuseSignals RetentionClass = "abuse_signals"
	// RetentionClassBilling covers the payment, subscription, refund and
	// reconciliation records kept as evidence of money and of the
	// reconciliation duty. They are retained and are never purged.
	RetentionClassBilling RetentionClass = "billing"
)

// RetentionAction is what the policy does with a governed class.
type RetentionAction string

const (
	// RetentionActionPurge removes records that no longer have a purpose.
	RetentionActionPurge RetentionAction = "purge"
	// RetentionActionAnonymize strips restricted references while keeping
	// the record.
	RetentionActionAnonymize RetentionAction = "anonymize"
	// RetentionActionRetain keeps every record under an obligation and
	// reports how many were kept.
	RetentionActionRetain RetentionAction = "retain"
)

// Retention windows: how long after a record became terminal the schedule
// applies. They are the policy decision this task makes; docs/PRIVACY.md §5
// publishes them and every change here is a policy change.
const (
	// RetentionTokensWindow is the grace period after a token is used or
	// expires. Thirty days cover support and incident correlation without
	// keeping a dead capability indefinitely.
	RetentionTokensWindow = 30 * 24 * time.Hour
	// RetentionSessionsWindow is the grace period after a session is
	// revoked or expires. It matches the token window: a dead session has
	// the same forensic value and the same cost of being wrong.
	RetentionSessionsWindow = 30 * 24 * time.Hour
	// RetentionAbuseSignalsWindow is how long the client IP and user agent
	// of a terminal session are kept. Prevention review happens while an
	// incident is fresh; a week is the declared limit, after which the
	// references are stripped and cannot be recovered.
	RetentionAbuseSignalsWindow = 7 * 24 * time.Hour
	// RetentionExportsWindow is one export lifetime: a document is purged
	// one lifetime after its link expired, and a request that was never
	// generated is expired one lifetime after it was made.
	RetentionExportsWindow = ExportTTL
)

// RetentionSchedule is one row of the executable policy: which class is
// governed, what happens to it and after how long.
type RetentionSchedule struct {
	// Class is the governed data class.
	Class RetentionClass
	// Action is what the job does with the class.
	Action RetentionAction
	// Window is the period that must elapse after a record became terminal
	// before the action applies. Zero means the action applies at the
	// terminal instant itself.
	Window time.Duration
	// Indefinite marks a class retained without a purge horizon under a
	// legal, contractual or evidentiary obligation: Window is zero and the
	// class never yields a cutoff.
	Indefinite bool
	// ReasonCode is the stable justification published with the policy.
	ReasonCode string
}

// retentionSchedules is the policy, in the fixed order the job enforces it
// (tokens, sessions, the retained trail, exports, prevention references and
// billing). Ordering is deterministic so a run is reproducible.
var retentionSchedules = []RetentionSchedule{
	{Class: RetentionClassTokens, Action: RetentionActionPurge, Window: RetentionTokensWindow, ReasonCode: "single_use_credential"},
	{Class: RetentionClassSessions, Action: RetentionActionPurge, Window: RetentionSessionsWindow, ReasonCode: "terminal_session"},
	{Class: RetentionClassReferentialLogs, Action: RetentionActionRetain, Indefinite: true, ReasonCode: "administrative_evidence"},
	{Class: RetentionClassExports, Action: RetentionActionPurge, Window: RetentionExportsWindow, ReasonCode: "expired_export_document"},
	{Class: RetentionClassAbuseSignals, Action: RetentionActionAnonymize, Window: RetentionAbuseSignalsWindow, ReasonCode: "prevention_reference"},
	{Class: RetentionClassBilling, Action: RetentionActionRetain, Indefinite: true, ReasonCode: "financial_evidence"},
}

// RetentionSchedules returns a copy of the policy, so a caller can read it
// but never mutate the schedule enforced by the job.
func RetentionSchedules() []RetentionSchedule {
	return append([]RetentionSchedule(nil), retentionSchedules...)
}

// ParseRetentionClass resolves a stored class value.
func ParseRetentionClass(value string) (RetentionClass, bool) {
	for _, schedule := range retentionSchedules {
		if string(schedule.Class) == value {
			return schedule.Class, true
		}
	}
	return "", false
}

// RetentionScheduleFor resolves the schedule of one class.
func RetentionScheduleFor(class RetentionClass) (RetentionSchedule, bool) {
	for _, schedule := range retentionSchedules {
		if schedule.Class == class {
			return schedule, true
		}
	}
	return RetentionSchedule{}, false
}

// IsValid verifies that a schedule is coherent: a known class, a known
// action, a non-negative window, a window exactly when the class has a
// purge horizon, and a stable reason code. An incoherent schedule is refused
// instead of being enforced approximately.
func (s RetentionSchedule) IsValid() error {
	if _, known := RetentionScheduleFor(s.Class); !known {
		return ErrUnknownRetentionClass
	}
	switch s.Action {
	case RetentionActionPurge, RetentionActionAnonymize, RetentionActionRetain:
	default:
		return ErrInvalidRetentionSchedule
	}
	if s.Window < 0 {
		return ErrInvalidRetentionSchedule
	}
	if strings.TrimSpace(s.ReasonCode) == "" {
		return ErrInvalidRetentionSchedule
	}
	if s.Indefinite {
		if s.Action != RetentionActionRetain || s.Window != 0 {
			return ErrInvalidRetentionSchedule
		}
		return nil
	}
	// A class with a purge horizon cannot be retained: retaining is the
	// absence of a boundary.
	if s.Action == RetentionActionRetain {
		return ErrInvalidRetentionSchedule
	}
	return nil
}

// HasCutoff reports whether the class has a purge or anonymize horizon. A
// retained class has none: nothing is ever removed, so there is no boundary
// to date.
func (s RetentionSchedule) HasCutoff() bool {
	return !s.Indefinite
}

// DueAt is the instant at which the action becomes due for a record that
// became terminal at terminalAt. A retained class has no due instant.
func (s RetentionSchedule) DueAt(terminalAt time.Time) (time.Time, bool) {
	if !s.HasCutoff() {
		return time.Time{}, false
	}
	return terminalAt.UTC().Add(s.Window), true
}

// Due reports whether a record that became terminal at terminalAt is due at
// now. The boundary instant is due: a record terminal exactly one window ago
// is no longer inside the window.
func (s RetentionSchedule) Due(terminalAt, now time.Time) bool {
	dueAt, hasDue := s.DueAt(terminalAt)
	if !hasDue {
		return false
	}
	return !now.UTC().Before(dueAt)
}

// Cutoff is the terminal boundary a run must pass at now: records terminal
// at or before the boundary are inside the class window. A retained class
// yields no cutoff.
func (s RetentionSchedule) Cutoff(now time.Time) (time.Time, bool) {
	if !s.HasCutoff() {
		return time.Time{}, false
	}
	return now.UTC().Add(-s.Window), true
}

// RetentionHold is an active legal or contractual hold over one class: the
// whole class when AccountID is empty, otherwise the records of one account.
// Holds are never erased; only their release changes them.
type RetentionHold struct {
	// ID is the stable hold identifier.
	ID string
	// Class is the held data class.
	Class RetentionClass
	// AccountID is the held account, empty for a whole-class hold.
	AccountID string
	// ReasonCode is the stable justification of the hold.
	ReasonCode string
	// PlacedAt is when the hold entered force.
	PlacedAt time.Time
}

// IsClassWide reports whether the hold covers the whole class.
func (h RetentionHold) IsClassWide() bool {
	return strings.TrimSpace(h.AccountID) == ""
}

// Validate verifies that a stored hold is coherent: a known class, a
// non-empty bounded reason code and a placement instant.
func (h RetentionHold) Validate() error {
	if _, known := ParseRetentionClass(string(h.Class)); !known {
		return ErrUnknownRetentionClass
	}
	reason := strings.TrimSpace(h.ReasonCode)
	if reason == "" || len(reason) > 100 {
		return ErrInvalidRetentionHold
	}
	if h.PlacedAt.IsZero() {
		return ErrInvalidRetentionHold
	}
	return nil
}
