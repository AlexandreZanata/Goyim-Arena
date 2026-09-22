package observability

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock is the deterministic clock of the package tests: nothing here
// sleeps on wall time, and a provider timestamp is reproducible.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

// newTestClock returns the shared instant of a test.
func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)}
}

// Now implements ports.Clock.
func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

// advance moves the clock forward.
func (clock *testClock) advance(elapsed time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(elapsed)
	clock.mu.Unlock()
}

// recordingSink remembers the events it was handed.
type recordingSink struct {
	mu     sync.Mutex
	events []Event
}

// Capture implements EventSink.
func (sink *recordingSink) Capture(event Event) {
	sink.mu.Lock()
	sink.events = append(sink.events, event)
	sink.mu.Unlock()
}

// captured copies the received events.
func (sink *recordingSink) captured() []Event {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]Event(nil), sink.events...)
}

// testLogger writes nowhere, so a test does not print what it asserts.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestNewRequiresLoggerAndClock covers the fail-closed composition: telemetry
// that cannot stamp or record is refused at boot, not at the first event.
func TestNewRequiresLoggerAndClock(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{Clock: newTestClock()}); err == nil {
		t.Fatal("a composition without a logger must be refused")
	}
	if _, err := New(Config{Logger: testLogger()}); err == nil {
		t.Fatal("a composition without a clock must be refused")
	}
}

// TestNewRefusesImpossibleSampleRate covers the sampling edge: a rate outside
// 0..100 is a configuration mistake, not a knob to clamp silently.
func TestNewRefusesImpossibleSampleRate(t *testing.T) {
	t.Parallel()

	for _, rate := range []int{-1, 101} {
		if _, err := New(Config{Logger: testLogger(), Clock: newTestClock(), SampleRatePercent: rate}); err == nil {
			t.Fatalf("sample rate %d must be refused", rate)
		}
	}
}

// TestNewWithoutCredentialsRecordsNothingButStaysUsable covers the default: a
// process with no provider credential still composes, and the analytics front
// is safe to call — it simply hands to no sink.
func TestNewWithoutCredentialsRecordsNothingButStaysUsable(t *testing.T) {
	t.Parallel()

	telemetry, err := New(Config{Logger: testLogger(), Clock: newTestClock()})
	if err != nil {
		t.Fatalf("compose without credentials: %v", err)
	}
	defer telemetry.Close()

	telemetry.Events.Capture(Event{Name: EventAccountSignedIn, AccountID: "acc-1", Properties: map[string]any{"locale": "pt-BR"}})
	telemetry.Errors.Report(ErrorReport{Message: "nothing to send"})

	if telemetry.Metrics == nil {
		t.Fatal("the metrics registry must always exist")
	}
}

// TestNewRefusesMalformedDSN covers fail-closed provider configuration: a DSN
// that does not parse refuses the composition instead of dropping every
// report at runtime.
func TestNewRefusesMalformedDSN(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Logger: testLogger(), Clock: newTestClock(), SentryDSN: "not-a-dsn"})
	if err == nil {
		t.Fatal("a malformed DSN must be refused")
	}
}

// TestHTTPMiddlewareObservesRequests covers RED: every request counts once by
// method, matched route and status, and its duration is observed.
func TestHTTPMiddlewareObservesRequests(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	telemetry := &Telemetry{Metrics: metrics, Errors: nopReporter{}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /thing/{id}", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})

	recorder := httptest.NewRecorder()
	telemetry.HTTPMiddleware(mux).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/thing/42", nil))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	rendered := metrics.Render()
	want := `http_requests_total{method="GET",route="/thing/{id}",status="201"} 1`
	if !strings.Contains(rendered, want) {
		t.Fatalf("exposition is missing %q:\n%s", want, rendered)
	}
	if !strings.Contains(rendered, `http_request_duration_seconds_count{method="GET",route="/thing/{id}"} 1`) {
		t.Fatalf("the request duration was not observed:\n%s", rendered)
	}
}

// TestHTTPMiddlewareNamesUnmatchedRequests covers the route label: a path no
// route matched is one series, never one series per stranger's URL.
func TestHTTPMiddlewareNamesUnmatchedRequests(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	telemetry := &Telemetry{Metrics: metrics, Errors: nopReporter{}}

	mux := http.NewServeMux()
	telemetry.HTTPMiddleware(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nothing/here", nil))

	if !strings.Contains(metrics.Render(), `route="unmatched"`) {
		t.Fatalf("an unmatched request must be labeled unmatched:\n%s", metrics.Render())
	}
}

// TestHTTPMiddlewareReportsPanicAndRethrows covers the error report of an
// unhandled panic: it is counted as a 500, reported once, and re-thrown so
// net/http still recovers it.
func TestHTTPMiddlewareReportsPanicAndRethrows(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	sink := &recordingReporter{}
	telemetry := &Telemetry{Metrics: metrics, Errors: sink}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	})

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("the panic must be re-thrown after being reported")
			}
		}()
		telemetry.HTTPMiddleware(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))
	}()

	reports := sink.reports()
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if reports[0].Kind != "panic" || reports[0].Operation != "GET /boom" {
		t.Fatalf("report = %+v, want a panic on GET /boom", reports[0])
	}
	if !strings.Contains(metrics.Render(), `status="500"`) {
		t.Fatalf("a panicked handler must be counted as a 500:\n%s", metrics.Render())
	}
}

// recordingReporter remembers the reports it was handed.
type recordingReporter struct {
	mu     sync.Mutex
	events []ErrorReport
}

// Report implements ErrorReporter.
func (reporter *recordingReporter) Report(report ErrorReport) {
	reporter.mu.Lock()
	reporter.events = append(reporter.events, report)
	reporter.mu.Unlock()
}

// reports copies the received reports.
func (reporter *recordingReporter) reports() []ErrorReport {
	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	return append([]ErrorReport(nil), reporter.events...)
}
