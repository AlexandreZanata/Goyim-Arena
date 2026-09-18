package domain

import (
	"strings"
	"time"
)

// Lease is the exclusive right of one worker to run one job until an instant.
type Lease struct {
	Owner string
	Until time.Time
}

// NewLease validates and builds a lease.
func NewLease(owner string, until time.Time) (Lease, error) {
	lease := Lease{Owner: owner, Until: until}
	if err := lease.Validate(); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

// Validate mirrors the database lease CHECKs: a holder is a non-empty,
// bounded identity and an expiry is a real instant; both exist together or
// neither does.
func (l Lease) Validate() error {
	if strings.TrimSpace(l.Owner) == "" || len(l.Owner) > MaxLeaseOwnerLength {
		return ErrInvalidLease
	}
	if l.Until.IsZero() {
		return ErrInvalidLease
	}
	return nil
}

// Job is one unit of durable work. It is the domain view of a queue row and
// mirrors the database CHECKs, so an incoherent job is refused before it
// reaches a statement.
type Job struct {
	ID              string
	Type            JobType
	Version         int
	Payload         []byte
	IdempotencyKey  string
	State           JobState
	AvailableAt     time.Time
	Attempts        int
	MaxAttempts     int
	LeaseOwner      string
	LeasedUntil     time.Time
	LastErrorCode   FailureCode
	LastErrorDetail string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Validate checks the whole entity against the invariants the schema enforces.
func (j Job) Validate() error {
	if strings.TrimSpace(j.ID) == "" {
		return ErrEmptyJobID
	}
	if !j.Type.IsValid() {
		return ErrUnknownJobType
	}
	if j.Version < 1 {
		return ErrInvalidVersion
	}
	if err := ValidatePayload(j.Payload); err != nil {
		return err
	}
	if !j.State.IsValid() {
		return ErrUnknownJobState
	}
	if j.MaxAttempts < 1 || j.Attempts < 0 || j.Attempts > j.MaxAttempts {
		return ErrInvalidAttempts
	}
	if err := j.validateLease(); err != nil {
		return err
	}
	if err := j.validateIdempotencyKey(); err != nil {
		return err
	}
	if err := j.validateFailure(); err != nil {
		return err
	}
	if j.CreatedAt.IsZero() || j.UpdatedAt.Before(j.CreatedAt) {
		return ErrInvalidTimestamps
	}
	return nil
}

// validateLease enforces that a lease exists exactly in the leased state, that
// it is coherent, and that a terminal job can never carry one.
func (j Job) validateLease() error {
	hasOwner := j.LeaseOwner != ""
	hasExpiry := !j.LeasedUntil.IsZero()
	if hasOwner != hasExpiry {
		return ErrInvalidLease
	}
	leased := j.State == StateLeased
	if leased != (hasOwner && hasExpiry) {
		return ErrLeaseIncoherent
	}
	if hasOwner {
		return Lease{Owner: j.LeaseOwner, Until: j.LeasedUntil}.Validate()
	}
	if j.State.IsTerminal() && (hasOwner || hasExpiry) {
		return ErrLeaseIncoherent
	}
	return nil
}

func (j Job) validateIdempotencyKey() error {
	if j.IdempotencyKey == "" {
		return nil
	}
	if strings.TrimSpace(j.IdempotencyKey) == "" || len(j.IdempotencyKey) > MaxIdempotencyKeyLength {
		return ErrInvalidIdempotencyKey
	}
	return nil
}

// validateFailure enforces the redacted-error coherence: code and detail
// appear together, the code is part of the closed vocabulary and the detail is
// bounded (the sanitizer is what makes its content safe).
func (j Job) validateFailure() error {
	hasCode := j.LastErrorCode != ""
	hasDetail := j.LastErrorDetail != ""
	if hasCode != hasDetail {
		return ErrInvalidFailure
	}
	if !hasCode {
		return nil
	}
	if !j.LastErrorCode.IsValid() {
		return ErrUnknownFailureCode
	}
	if len(j.LastErrorDetail) > MaxErrorDetailLength {
		return ErrInvalidFailure
	}
	return nil
}

// IsTerminal reports whether the job is a recorded outcome.
func (j Job) IsTerminal() bool { return j.State.IsTerminal() }

// LeaseExpired reports whether the job holds a lease that already ended at
// now, which makes it claimable again by any worker.
func (j Job) LeaseExpired(now time.Time) bool {
	if j.State != StateLeased || j.LeasedUntil.IsZero() {
		return false
	}
	return !now.Before(j.LeasedUntil)
}

// WithinAttemptBudget reports whether the job may run again: the attempt is
// consumed at lease time, so a job that already spent its budget is dead.
func (j Job) WithinAttemptBudget() bool { return j.Attempts < j.MaxAttempts }

// Lease returns the job lease. It is invalid when the job holds none, so the
// caller must check the state first (mirrors the schema coherence).
func (j Job) Lease() Lease {
	return Lease{Owner: j.LeaseOwner, Until: j.LeasedUntil}
}

// Failure returns the redacted failure recorded on the job, if any.
func (j Job) Failure() (Failure, bool) {
	if j.LastErrorCode == "" {
		return Failure{}, false
	}
	return Failure{Code: j.LastErrorCode, Detail: j.LastErrorDetail}, true
}
