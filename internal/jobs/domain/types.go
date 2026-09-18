package domain

// JobState is the closed lifecycle vocabulary of one unit of work.
//
// queued    the job is runnable once available_at is reached;
// leased    one worker holds the job until leased_until;
// succeeded the job finished; the row is a recorded outcome;
// dead      the job exhausted its attempt budget; it left the queue.
type JobState string

const (
	StateQueued    JobState = "queued"
	StateLeased    JobState = "leased"
	StateSucceeded JobState = "succeeded"
	StateDead      JobState = "dead"
)

// AllJobStates lists the lifecycle in a deterministic order, so tests can pin
// the vocabulary and the orchestrator can iterate it exhaustively.
var AllJobStates = []JobState{StateQueued, StateLeased, StateSucceeded, StateDead}

// IsValid reports whether the state belongs to the lifecycle vocabulary.
func (s JobState) IsValid() bool {
	switch s {
	case StateQueued, StateLeased, StateSucceeded, StateDead:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the state is a recorded outcome that no lease
// can return to the queue.
func (s JobState) IsTerminal() bool {
	return s == StateSucceeded || s == StateDead
}

// JobType is the closed workload vocabulary. The queue stores the workload
// name and not a handler: the worker resolves the name to a versioned handler
// (P15-T02), so a row is readable by any component and cannot smuggle code.
type JobType string

const (
	TypeEmailDelivery         JobType = "email_delivery"
	TypeINKGrantMonthly       JobType = "ink_grant_monthly"
	TypePassExpiry            JobType = "pass_expiry"
	TypeRetentionRun          JobType = "retention_run"
	TypeSessionCleanup        JobType = "session_cleanup"
	TypeBillingReconciliation JobType = "billing_reconciliation"
)

// AllJobTypes lists the workload vocabulary in a deterministic order.
var AllJobTypes = []JobType{
	TypeEmailDelivery,
	TypeINKGrantMonthly,
	TypePassExpiry,
	TypeRetentionRun,
	TypeSessionCleanup,
	TypeBillingReconciliation,
}

// IsValid reports whether the type belongs to the closed vocabulary.
func (t JobType) IsValid() bool {
	for _, known := range AllJobTypes {
		if t == known {
			return true
		}
	}
	return false
}

// Maximum sizes accepted by the queue. They mirror the database CHECKs, so the
// domain refuses a payload the database would also refuse instead of failing
// later inside a statement.
const (
	// MaxPayloadBytes bounds the serialized payload.
	MaxPayloadBytes = 4096
	// MaxPayloadDepth bounds the nesting: one object plus one nested
	// container, enough for identifiers and lists of parameters.
	MaxPayloadDepth = 2
	// MaxLeaseOwnerLength bounds the worker identity.
	MaxLeaseOwnerLength = 128
	// MaxIdempotencyKeyLength bounds a caller-chosen retry key.
	MaxIdempotencyKeyLength = 200
	// MaxErrorDetailLength bounds the redacted failure detail.
	MaxErrorDetailLength = 300
	// DefaultMaxAttempts is the attempt budget when the caller omits one.
	DefaultMaxAttempts = 5
)
