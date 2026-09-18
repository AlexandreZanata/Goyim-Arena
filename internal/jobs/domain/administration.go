package domain

import (
	"errors"
	"strings"
	"time"
)

// StepUpWindow bounds how old the operator's authentication may be before an
// operational action is refused.
//
// The rule mirrors the moderation module's: a sensitive action requires a
// recent authentication, and "recent" is a policy constant, not something a
// client states. Fifteen minutes is short enough that a stolen cookie left
// open on a desk is unlikely to be within the window, and long enough that an
// operator investigating a queue does not have to re-authenticate between two
// retries.
const StepUpWindow = 15 * time.Minute

// Action names for the audit trail. The audit domain requires the
// `<subject>.<verb>` shape, lowercase, and what an operator did is exactly
// what an auditor needs to read years later.
const (
	// ActionRetryDeadJob is one authorized retry of a dead job.
	ActionRetryDeadJob = "jobs.retry"
)

// ReasonCodeDeadJobRetry is the stable reason code recorded for one
// authorized retry.
//
// The trail's contract is explicit that its reason column carries a stable
// code or rule and that free prose never enters it, so the operator's stated
// justification does not travel here: it travels as the allowlisted `reason`
// metadata key, exactly as the wallet adjustment, the arena moderation and the
// attribution moderation already do. This constant is what makes the column
// greppable across modules years later.
const ReasonCodeDeadJobRetry = "dead_job_retry"

// Errors of the administrative surface.
var (
	// ErrStepUpRequired reports an authentication that is too old for the
	// action.
	ErrStepUpRequired = errors.New("jobs: sensitive action requires recent authentication")
	// ErrRetryNotAllowed reports a workload outside the retry allowlist.
	ErrRetryNotAllowed = errors.New("jobs: workload is not retryable")
	// ErrJobNotDead reports a retry of a job that is not in the dead state.
	ErrJobNotDead = errors.New("jobs: job is not dead")
	// ErrEmptyReason reports a retry without a stated reason.
	ErrEmptyReason = errors.New("jobs: retry requires a reason")
	// ErrEmptyActor reports an administrative action without an actor.
	ErrEmptyActor = errors.New("jobs: administrative action requires an actor")
)

// retryableTypes is the closed allowlist of workloads an operator may retry.
//
// It is an allowlist and not a denylist on purpose: a workload added to the
// queue later is not retryable until someone decides it is, which is the safe
// default for a path that runs a handler by hand. Every maintenance workload
// is here because the scheduler queues it with a period and the pass is
// idempotent, so re-running one period is exactly what the queue is for;
// email delivery is here because a provider outage is the archetypal reason a
// message is dead.
var retryableTypes = []JobType{
	TypeBillingReconciliation,
	TypeEmailDelivery,
	TypeINKGrantMonthly,
	TypePassExpiry,
	TypeRetentionRun,
	TypeSessionCleanup,
}

// RetryableJobTypes lists the allowlist in a deterministic order.
func RetryableJobTypes() []JobType {
	out := make([]JobType, len(retryableTypes))
	copy(out, retryableTypes)
	return out
}

// RetryAllowed reports whether the workload may be retried by an operator.
func RetryAllowed(jobType JobType) bool {
	for _, allowed := range retryableTypes {
		if jobType == allowed {
			return true
		}
	}
	return false
}

// StepUpSatisfied reports whether an action may proceed given the age of the
// session that is asking.
//
// A non-positive age is not "very fresh": it means the age could not be
// determined, so the action is refused. An operator action that cannot prove
// its freshness is exactly the case the rule exists for.
func StepUpSatisfied(action string, sessionAge time.Duration) (bool, error) {
	if strings.TrimSpace(action) == "" {
		return false, ErrStepUpRequired
	}
	if sessionAge <= 0 || sessionAge > StepUpWindow {
		return false, ErrStepUpRequired
	}
	return true, nil
}

// ValidateReason bounds the operator's stated reason.
//
// The reason is recorded in the audit trail and shown to nobody, so it is
// bounded rather than sanitized here: it never reaches a client, a log line or
// a job row, and the audit metadata allowlist is what keeps it from carrying
// anything else.
func ValidateReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || len(trimmed) > 200 {
		return ErrEmptyReason
	}
	return nil
}
