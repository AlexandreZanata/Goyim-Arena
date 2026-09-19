package dbbudget

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTrackerCountsQueriesAndLatency(t *testing.T) {
	tracker := NewTracker()
	ctx := WithTracker(context.Background(), tracker)
	if FromContext(ctx) != tracker {
		t.Fatal("tracker was not attached to context")
	}
	tracker.Start()
	tracker.Observe(3 * time.Millisecond)
	tracker.Start()
	tracker.Observe(5 * time.Millisecond)
	observation := tracker.Observation()
	if observation.Queries != 2 || observation.Latency != 8*time.Millisecond {
		t.Fatalf("observation = %+v, want 2 queries and 8ms", observation)
	}
	if err := tracker.Assert(Budget{Name: "feed", MaxQueries: 2, MaxLatency: 8 * time.Millisecond}); err != nil {
		t.Fatalf("Assert() error = %v", err)
	}
	if err := tracker.Assert(Budget{Name: "feed", MaxQueries: 1, MaxLatency: time.Second}); err == nil {
		t.Fatal("query-count overflow must fail")
	}
	if err := tracker.Assert(Budget{Name: "feed", MaxQueries: 2, MaxLatency: 7 * time.Millisecond}); err == nil {
		t.Fatal("latency overflow must fail")
	}
}

func TestTrackerIsSafeForConcurrentRequests(t *testing.T) {
	tracker := NewTracker()
	const workers = 16
	const queries = 10
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := 0; i < queries; i++ {
				tracker.Start()
				tracker.Observe(time.Microsecond)
			}
		}()
	}
	group.Wait()
	observation := tracker.Observation()
	if observation.Queries != workers*queries || observation.Latency != workers*queries*time.Microsecond {
		t.Fatalf("observation = %+v, want %d queries and %s", observation, workers*queries, time.Duration(workers*queries)*time.Microsecond)
	}
}

func TestBudgetRejectsIncompleteDefinitions(t *testing.T) {
	tracker := NewTracker()
	for _, budget := range []Budget{{}, {Name: "x", MaxQueries: 1}, {Name: "x", MaxLatency: time.Second}} {
		if err := tracker.Assert(budget); err == nil {
			t.Fatalf("budget %+v unexpectedly passed", budget)
		}
	}
}
