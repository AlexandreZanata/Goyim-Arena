package observability

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The bounds of the instrumentation.
const (
	// queueScrapeTimeout bounds the database read behind the queue gauges.
	// A scrape that cannot answer promptly reports nothing, it does not
	// hold the exposition open.
	queueScrapeTimeout = 2 * time.Second
	// queueCacheTTL is how long one queue reading serves the whole scrape.
	// The three gauge families share it so a scrape costs one query, not
	// three.
	queueCacheTTL = 5 * time.Second
)

// httpMiddleware observes every request and optionally reports a panic.
//
// RED vocabulary: every request counts once by method, matched route and
// status (rate and errors); its duration is observed by method and route
// (duration). The route label is the mux pattern — a closed, registered
// vocabulary — and never the raw path, which would let a stranger mint one
// metric series per URL.
func (metrics *Metrics) httpMiddleware(next http.Handler, reporter ErrorReporter) http.Handler {
	if metrics == nil {
		return next
	}
	metrics.httpOnce.Do(func() {
		metrics.GaugeFunc("http_requests_in_flight", "HTTP requests currently being served.", func() []GaugeSample {
			return []GaugeSample{{Value: float64(metrics.inFlight.Load())}}
		})
	})

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := metrics.clock.Now()
		metrics.inFlight.Add(1)
		recorder := &statusRecorder{ResponseWriter: writer}

		defer func() {
			recovered := recover()
			if recovered != nil {
				recorder.forceStatus(http.StatusInternalServerError)
				if reporter != nil {
					reporter.Report(ErrorReport{
						Message:   "http handler panic",
						Kind:      "panic",
						Operation: requestOperation(request),
						RequestID: requestCorrelation(request),
					})
				}
			}

			metrics.inFlight.Add(-1)
			route := requestRoute(request)
			metrics.Counter("http_requests_total", "HTTP requests served, by method, matched route and status.", map[string]string{
				"method": request.Method,
				"route":  route,
				"status": strconv.Itoa(recorder.statusCode()),
			}).Inc()
			metrics.Histogram("http_request_duration_seconds", "HTTP request duration in seconds, by method and matched route.", map[string]string{
				"method": request.Method,
				"route":  route,
			}).Observe(metrics.clock.Now().Sub(start).Seconds())

			if recovered != nil {
				panic(recovered)
			}
		}()

		next.ServeHTTP(recorder, request)
	})
}

// statusRecorder captures the status a handler writes without changing the
// response, and exposes the wrapped writer through Unwrap so the
// ResponseController keeps reaching the original.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records and forwards the status.
func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.record(status)
	recorder.ResponseWriter.WriteHeader(status)
}

// Write records the implicit 200 of a handler that never called WriteHeader.
func (recorder *statusRecorder) Write(body []byte) (int, error) {
	recorder.record(http.StatusOK)
	return recorder.ResponseWriter.Write(body)
}

// Unwrap exposes the wrapped writer to http.ResponseController.
func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

// record stores the first status written.
func (recorder *statusRecorder) record(status int) {
	if recorder.status == 0 {
		recorder.status = status
	}
}

// forceStatus overrides the recorded status, for the panic path: a handler
// that panicked was not a success, however far into the response it got.
func (recorder *statusRecorder) forceStatus(status int) {
	recorder.status = status
}

// statusCode reads the final status. A handler that wrote nothing is a 200:
// net/http answers an empty 200 for it, and the metric must say what the
// client saw.
func (recorder *statusRecorder) statusCode() int {
	if recorder.status == 0 {
		return http.StatusOK
	}
	return recorder.status
}

// requestRoute renders the matched mux pattern without its method prefix, or
// "unmatched" when the mux had no route for the path.
func requestRoute(request *http.Request) string {
	pattern := request.Pattern
	if pattern == "" {
		return "unmatched"
	}
	if _, path, found := strings.Cut(pattern, " "); found {
		return path
	}
	return pattern
}

// ObserveJob records one finished job: it counts by type and outcome (rate
// and errors) and observes its duration by type.
func (metrics *Metrics) ObserveJob(jobType, outcome string, elapsed time.Duration) {
	if metrics == nil {
		return
	}
	metrics.Counter("jobs_processed_total", "Jobs finished by the worker, by type and outcome.", map[string]string{
		"type":    jobType,
		"outcome": outcome,
	}).Inc()
	metrics.Histogram("jobs_processing_seconds", "Job duration in seconds, by type.", map[string]string{
		"type": jobType,
	}).Observe(elapsed.Seconds())
}

// PoolStats is the USE view of the database connection pool, read from
// pgxpool.Stat by the composition root. It carries counters already
// accumulated by the library; the process does not keep a second copy.
type PoolStats struct {
	Total      int32
	Acquired   int32
	Idle       int32
	Max        int32
	Acquires   int64
	Empty      int64
	Canceled   int64
	AcquireSum float64
}

// RegisterPool exposes the pool as utilization, saturation and errors:
// connections by state, how often an acquire found the pool empty (the
// saturation signal), how often one was canceled (the error signal) and the
// cumulative wait.
func (metrics *Metrics) RegisterPool(read func() PoolStats) {
	if metrics == nil || read == nil {
		return
	}
	metrics.GaugeFunc("db_pool_connections", "Database pool connections by state.", func() []GaugeSample {
		stats := read()
		return []GaugeSample{
			{Labels: map[string]string{"state": "total"}, Value: float64(stats.Total)},
			{Labels: map[string]string{"state": "acquired"}, Value: float64(stats.Acquired)},
			{Labels: map[string]string{"state": "idle"}, Value: float64(stats.Idle)},
			{Labels: map[string]string{"state": "max"}, Value: float64(stats.Max)},
		}
	})
	metrics.CounterFunc("db_pool_acquires_total", "Database pool acquisitions.", func() float64 {
		return float64(read().Acquires)
	})
	metrics.CounterFunc("db_pool_empty_acquires_total", "Database pool acquisitions that found the pool empty (saturation).", func() float64 {
		return float64(read().Empty)
	})
	metrics.CounterFunc("db_pool_canceled_acquires_total", "Database pool acquisitions canceled waiting for a connection (errors).", func() float64 {
		return float64(read().Canceled)
	})
	metrics.CounterFunc("db_pool_acquire_seconds_total", "Cumulative database pool wait in seconds.", func() float64 {
		return read().AcquireSum
	})
}

// QueueStats is the USE view of the durable job queue.
type QueueStats struct {
	Queued         int64
	Leased         int64
	DueNow         int64
	Dead           int64
	LagSeconds     int64
	OldestDeadSecs int64
}

// RegisterQueue exposes the queue as utilization, saturation and errors: the
// counts by state, how much work is runnable now (saturation), the wait of
// the oldest due job (the lag an alert watches) and the age of the oldest
// dead one. The three families share one cached reading per scrape.
func (metrics *Metrics) RegisterQueue(read func(ctx context.Context) (QueueStats, error)) {
	if metrics == nil || read == nil {
		return
	}

	scrapeErrors := metrics.Counter("jobs_health_scrape_errors_total", "Queue health readings the database refused.", nil)

	var mu sync.Mutex
	var cached QueueStats
	var cachedErr error
	var sampledAt time.Time
	var sampled bool

	load := func() (QueueStats, error) {
		mu.Lock()
		defer mu.Unlock()
		now := metrics.clock.Now()
		if sampled && now.Sub(sampledAt) < queueCacheTTL {
			return cached, cachedErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), queueScrapeTimeout)
		defer cancel()
		stats, err := read(ctx)
		sampledAt, sampled = now, true
		if err != nil {
			cachedErr = err
			scrapeErrors.Inc()
			return QueueStats{}, err
		}
		cached, cachedErr = stats, nil
		return cached, nil
	}

	metrics.GaugeFunc("jobs_queue", "Durable jobs by state.", func() []GaugeSample {
		stats, err := load()
		if err != nil {
			return nil
		}
		return []GaugeSample{
			{Labels: map[string]string{"state": "queued"}, Value: float64(stats.Queued)},
			{Labels: map[string]string{"state": "leased"}, Value: float64(stats.Leased)},
			{Labels: map[string]string{"state": "due_now"}, Value: float64(stats.DueNow)},
			{Labels: map[string]string{"state": "dead"}, Value: float64(stats.Dead)},
		}
	})
	metrics.GaugeFunc("jobs_lag_seconds", "Wait of the oldest due job, in seconds.", func() []GaugeSample {
		stats, err := load()
		if err != nil {
			return nil
		}
		return []GaugeSample{{Value: float64(stats.LagSeconds)}}
	})
	metrics.GaugeFunc("jobs_oldest_dead_seconds", "Age of the oldest dead job, in seconds.", func() []GaugeSample {
		stats, err := load()
		if err != nil {
			return nil
		}
		return []GaugeSample{{Value: float64(stats.OldestDeadSecs)}}
	})
}
