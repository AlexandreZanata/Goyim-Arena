package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMetricsRendersCounterGaugeAndHistogram covers the exposition format:
// metadata once, series sorted inside the family, cumulative histogram
// buckets ending at +Inf.
func TestMetricsRendersCounterGaugeAndHistogram(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	metrics.Counter("things_total", "Things.", map[string]string{"kind": "b"}).Inc()
	metrics.Counter("things_total", "Things.", map[string]string{"kind": "a"}).Add(3)
	metrics.GaugeFunc("things_live", "Live things.", func() []GaugeSample {
		return []GaugeSample{{Labels: map[string]string{"state": "open"}, Value: 7}}
	})
	metrics.Histogram("thing_seconds", "Thing duration.", nil).Observe(0.03)

	rendered := metrics.Render()
	for _, want := range []string{
		"# TYPE things_total counter",
		`things_total{kind="a"} 3`,
		`things_total{kind="b"} 1`,
		"# TYPE things_live gauge",
		`things_live{state="open"} 7`,
		"# TYPE thing_seconds histogram",
		"thing_seconds_bucket{le=\"+Inf\"} 1",
		"thing_seconds_sum",
		"thing_seconds_count 1",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("exposition is missing %q:\n%s", want, rendered)
		}
	}
	if strings.Index(rendered, `things_total{kind="a"}`) > strings.Index(rendered, `things_total{kind="b"}`) {
		t.Fatal("label sets must be rendered in a stable order")
	}
}

// TestMetricsHandlerServesOneScrape covers the administrative endpoint: it is
// a plain text exposition with the Prometheus content type.
func TestMetricsHandlerServesOneScrape(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	metrics.Counter("things_total", "Things.", nil).Inc()

	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("content type = %q, want the Prometheus text format", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "things_total 1") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

// TestMetricsNilHandlesAreInert covers the safe zero values of the handles:
// a surface that stored no counter must not panic when its code path runs.
func TestMetricsNilHandlesAreInert(t *testing.T) {
	t.Parallel()

	var counter *Counter
	counter.Inc()
	counter.Add(5)
	var histogram *Histogram
	histogram.Observe(0.5)

	// A registry that was never composed renders nothing and installs no
	// gauge, so a process without telemetry serves its pages.
	var metrics *Metrics
	metrics.RegisterPool(func() PoolStats { return PoolStats{} })
	metrics.RegisterQueue(func(context.Context) (QueueStats, error) { return QueueStats{}, nil })
	if metrics.Render() != "" {
		t.Fatal("a nil registry must render nothing")
	}

	var telemetry *Telemetry
	telemetry.Close()
	if telemetry.MetricsHandler() == nil {
		t.Fatal("a nil telemetry must still answer its metrics endpoint")
	}
	mux := http.NewServeMux()
	if handler := telemetry.HTTPMiddleware(mux); handler == nil {
		t.Fatal("a nil telemetry must leave the handler transparent")
	}
}

// TestRegisterPoolExposesStateSaturationAndErrors covers USE of the database
// pool: connections by state, plus the library's own acquire counters, so the
// process keeps no second copy that drifts.
func TestRegisterPoolExposesStateSaturationAndErrors(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	metrics.RegisterPool(func() PoolStats {
		return PoolStats{Total: 5, Acquired: 2, Idle: 3, Max: 10, Acquires: 40, Empty: 4, Canceled: 1, AcquireSum: 2.5}
	})

	rendered := metrics.Render()
	for _, want := range []string{
		`db_pool_connections{state="total"} 5`,
		`db_pool_connections{state="acquired"} 2`,
		`db_pool_connections{state="idle"} 3`,
		`db_pool_connections{state="max"} 10`,
		"db_pool_acquires_total 40",
		"db_pool_empty_acquires_total 4",
		"db_pool_canceled_acquires_total 1",
		"db_pool_acquire_seconds_total 2.5",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("exposition is missing %q:\n%s", want, rendered)
		}
	}
}

// TestRegisterQueueExposesLagAndSharesOneReading covers USE of the job queue:
// the states, the lag an alert watches and the age of the oldest dead job,
// all served from one cached reading per scrape so a scrape costs one query.
func TestRegisterQueueExposesLagAndSharesOneReading(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	reads := 0
	metrics.RegisterQueue(func(context.Context) (QueueStats, error) {
		reads++
		return QueueStats{Queued: 4, Leased: 1, DueNow: 2, Dead: 3, LagSeconds: 90, OldestDeadSecs: 600}, nil
	})

	rendered := metrics.Render()
	for _, want := range []string{
		`jobs_queue{state="queued"} 4`,
		`jobs_queue{state="leased"} 1`,
		`jobs_queue{state="due_now"} 2`,
		`jobs_queue{state="dead"} 3`,
		"jobs_lag_seconds 90",
		"jobs_oldest_dead_seconds 600",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("exposition is missing %q:\n%s", want, rendered)
		}
	}
	if reads != 1 {
		t.Fatalf("the three gauge families shared %d readings, want 1", reads)
	}
}

// TestRegisterQueueCountsAScrapeError covers the failure path: a database that
// refuses the reading is counted and the family renders nothing rather than a
// stale or invented number.
func TestRegisterQueueCountsAScrapeError(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	metrics.RegisterQueue(func(context.Context) (QueueStats, error) {
		return QueueStats{}, errors.New("database refused the reading")
	})

	rendered := metrics.Render()
	if strings.Contains(rendered, "jobs_lag_seconds 0") {
		t.Fatalf("a failed reading must not be rendered as zero:\n%s", rendered)
	}
	if !strings.Contains(rendered, "jobs_health_scrape_errors_total 1") {
		t.Fatalf("the failed reading must be counted:\n%s", rendered)
	}
}

// TestMetricsCachesQueueReadingsWithinTheTTL covers the cost bound: two
// renders close together share one reading, and a reading older than the TTL
// is refreshed.
func TestMetricsCachesQueueReadingsWithinTheTTL(t *testing.T) {
	t.Parallel()

	clock := newTestClock()
	metrics := NewMetrics(clock)
	reads := 0
	metrics.RegisterQueue(func(context.Context) (QueueStats, error) {
		reads++
		return QueueStats{Queued: int64(reads)}, nil
	})

	metrics.Render()
	metrics.Render()
	if reads != 1 {
		t.Fatalf("readings within the TTL = %d, want 1", reads)
	}
	clock.advance(queueCacheTTL + time.Second)
	metrics.Render()
	if reads != 2 {
		t.Fatalf("readings after the TTL = %d, want 2", reads)
	}
}
