package domain

import "time"

// DeletionCooldown is the mandatory cooling-off period between requesting
// account deletion and its execution (P14-T06; docs/PRIVACY.md §4/§5,
// BR §10). The holder can cancel during the window; after it the workflow
// may execute the request.
const DeletionCooldown = 7 * 24 * time.Hour

// DeletionRequestStatus is the closed lifecycle vocabulary of one account
// deletion request: requested (cooling off, cancellable), executed (the
// account was anonymized) and canceled (the holder aborted it).
type DeletionRequestStatus string

const (
	// DeletionStatusRequested marks a request inside its cooldown window.
	DeletionStatusRequested DeletionRequestStatus = "requested"
	// DeletionStatusExecuted marks an executed deletion.
	DeletionStatusExecuted DeletionRequestStatus = "executed"
	// DeletionStatusCanceled marks a request canceled by the holder.
	DeletionStatusCanceled DeletionRequestStatus = "canceled"
)

// DeletionExecutable reports whether a requested deletion may execute at
// the given instant: the cooldown must have elapsed. A zero or future
// instant denies instead of executing early.
func DeletionExecutable(requestedAt, now time.Time) bool {
	if requestedAt.IsZero() || now.IsZero() {
		return false
	}
	return !now.UTC().Before(requestedAt.UTC().Add(DeletionCooldown))
}

// DeletionCancellable reports whether the holder may still cancel the
// request at the given instant: only while it is requested and inside the
// cooldown window.
func DeletionCancellable(status DeletionRequestStatus, now, requestedAt time.Time) bool {
	if status != DeletionStatusRequested {
		return false
	}
	return !DeletionExecutable(requestedAt, now)
}

// AnonymizationPolicy documents the stable behavior of deletion for public
// authorship (P14-T06; docs/PRIVACY.md §5, BR §10): the profile row and the
// username history are removed, so public content stays intact while its
// author becomes unresolvable. No placeholder handle is ever derived from
// the former username, so the anonymized state is not a re-identification
// vector; any localized display string lives in the interface catalogs, not
// in persisted state.
const AnonymizationPolicy = "unresolvable_author"
