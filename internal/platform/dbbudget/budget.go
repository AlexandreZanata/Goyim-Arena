// Package dbbudget provides request-scoped database query accounting for
// performance tests. It contains no database or HTTP dependency: the pgx
// tracer records observations when a tracker is present in a context.
package dbbudget

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type contextKey struct{}

// Tracker accumulates the number and elapsed time of queries issued with its
// context. It is safe for concurrent use so a request test may exercise
// concurrent work without racing the accounting itself.
type Tracker struct {
	mu      sync.Mutex
	queries int
	latency time.Duration
}

// Observation is the measured result of one request or hot-path operation.
type Observation struct {
	Queries int
	Latency time.Duration
}

// Budget fixes the maximum query count and elapsed database time of a hot
// path. A zero maximum is invalid; budgets must be explicit in tests.
type Budget struct {
	Name       string
	MaxQueries int
	MaxLatency time.Duration
}

// NewTracker creates an empty request tracker.
func NewTracker() *Tracker { return &Tracker{} }

// WithTracker attaches tracker to ctx. A nil tracker leaves ctx unchanged.
func WithTracker(ctx context.Context, tracker *Tracker) context.Context {
	if tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, tracker)
}

// FromContext retrieves the tracker attached to ctx, if any.
func FromContext(ctx context.Context) *Tracker {
	tracker, _ := ctx.Value(contextKey{}).(*Tracker)
	return tracker
}

// Start records one query before it is sent to PostgreSQL.
func (tracker *Tracker) Start() {
	tracker.mu.Lock()
	tracker.queries++
	tracker.mu.Unlock()
}

// Observe adds one completed query duration.
func (tracker *Tracker) Observe(duration time.Duration) {
	tracker.mu.Lock()
	tracker.latency += duration
	tracker.mu.Unlock()
}

// Observation returns a consistent count and cumulative database latency.
func (tracker *Tracker) Observation() Observation {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return Observation{Queries: tracker.queries, Latency: tracker.latency}
}

// Assert checks a hot-path budget and returns a diagnostic suitable for a
// failing test. Both dimensions are required: a one-query scan can still
// violate the latency budget.
func (tracker *Tracker) Assert(budget Budget) error {
	if budget.Name == "" || budget.MaxQueries <= 0 || budget.MaxLatency <= 0 {
		return fmt.Errorf("dbbudget: invalid budget %+v", budget)
	}
	observation := tracker.Observation()
	if observation.Queries > budget.MaxQueries {
		return fmt.Errorf("dbbudget: %s used %d queries, budget is %d", budget.Name, observation.Queries, budget.MaxQueries)
	}
	if observation.Latency > budget.MaxLatency {
		return fmt.Errorf("dbbudget: %s used %s database time, budget is %s", budget.Name, observation.Latency, budget.MaxLatency)
	}
	return nil
}
