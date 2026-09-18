// Package application defines the use cases, orchestrations and
// consumer-oriented ports of the jobs module (P15-T01).
//
// The queue is durable: every state transition is a statement against
// app.jobs, never an in-memory structure. Use cases validate the domain rules
// and delegate the concurrency-critical part (claiming one due row with
// SKIP LOCKED) to the repository port, so two workers can never lease the same
// job.
package application

import "errors"

var (
	// ErrInvalidQueueConfig indicates a use case was built without its
	// required dependencies.
	ErrInvalidQueueConfig = errors.New("application: job queue configuration is invalid")

	// ErrJobNotFound indicates the job is unknown.
	ErrJobNotFound = errors.New("application: job not found")

	// ErrLeaseNotHeld indicates the caller does not hold the lease: the job is
	// not leased, or another worker holds it (a reclaimed lease included).
	// The stale holder must not overwrite the new outcome.
	ErrLeaseNotHeld = errors.New("application: job lease is not held by this worker")

	// ErrInvalidLeaseOwner indicates the worker identity is empty or too long.
	ErrInvalidLeaseOwner = errors.New("application: lease owner is invalid")

	// ErrInvalidLeaseDuration indicates the requested lease window is
	// non-positive or longer than the allowed maximum.
	ErrInvalidLeaseDuration = errors.New("application: lease duration is invalid")

	// ErrInvalidMaxAttempts indicates the enqueue command carries an attempt
	// budget below one.
	ErrInvalidMaxAttempts = errors.New("application: max attempts is invalid")

	// ErrInvalidWorkerConfig indicates the worker configuration is incoherent
	// (unbounded concurrency, a handler deadline that outlives its lease, a
	// non-positive poll interval or an unusable backoff policy).
	ErrInvalidWorkerConfig = errors.New("application: worker configuration is invalid")

	// ErrInvalidBackoff indicates the retry policy has no usable bounds.
	ErrInvalidBackoff = errors.New("application: backoff policy is invalid")

	// ErrInvalidHandler indicates a registration carries no handler function.
	ErrInvalidHandler = errors.New("application: job handler is nil")

	// ErrDuplicateHandler indicates the same workload and payload version were
	// registered twice; a silent replacement would change the meaning of jobs
	// already in the queue.
	ErrDuplicateHandler = errors.New("application: job handler is already registered")

	// ErrUnknownHandler indicates no handler exists for the workload at all.
	ErrUnknownHandler = errors.New("application: no handler for job type")

	// ErrUnsupportedHandlerVersion indicates the workload has handlers but not
	// for the payload version carried by the job: the deployment must be rolled
	// forward instead of guessing what the payload means.
	ErrUnsupportedHandlerVersion = errors.New("application: unsupported job payload version")
)
