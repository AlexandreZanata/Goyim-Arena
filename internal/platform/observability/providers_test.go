package observability

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestParseSentryDSNRemovesTheCredentialFromTheEndpoint covers the transport
// shape: the endpoint keeps the host and the numeric project and never carries
// the key, so a transport error cannot quote the credential.
func TestParseSentryDSNRemovesTheCredentialFromTheEndpoint(t *testing.T) {
	t.Parallel()

	host, project, key, err := parseSentryDSN("https://abc123@o1.ingest.sentry.io/42")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if host != "o1.ingest.sentry.io" || project != "42" || key != "abc123" {
		t.Fatalf("parsed (%q, %q, %q)", host, project, key)
	}

	endpoint, err := sentryEndpoint("https://"+host, project)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://o1.ingest.sentry.io/api/42/envelope/" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if strings.Contains(endpoint, key) {
		t.Fatal("the endpoint must not carry the credential")
	}
}

// TestSentryEndpointAcceptsOnlyAnOrigin covers the base rule: https
// everywhere, plain http only for a loopback fake, and never a path, query,
// fragment or credentials.
func TestSentryEndpointAcceptsOnlyAnOrigin(t *testing.T) {
	t.Parallel()

	if _, err := sentryEndpoint("http://127.0.0.1:8080", "1"); err != nil {
		t.Fatalf("a loopback fake must be accepted: %v", err)
	}
	for _, base := range []string{
		"http://o1.ingest.sentry.io",
		"https://o1.ingest.sentry.io/api",
		"https://o1.ingest.sentry.io?x=1",
		"not-a-url",
	} {
		if _, err := sentryEndpoint(base, "1"); err == nil {
			t.Fatalf("origin %q must be refused", base)
		}
	}
}

// TestParseSentryDSNRefusesMalformedShapes covers fail-closed configuration:
// every unusable DSN is refused at composition, naming the reason.
func TestParseSentryDSNRefusesMalformedShapes(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"",
		" abc@host/1",
		"http://abc@host/1",
		"https://host/1",
		"https://abc@host",
		"https://abc@host/not-a-number",
		"https://abc@host/1/extra",
		"https://abc@ /1",
	} {
		if _, _, _, err := parseSentryDSN(raw); err == nil {
			t.Fatalf("DSN %q must be refused", raw)
		}
	}
}

// TestNewSentryReporterRequiresAClock covers the composition: a reporter that
// cannot stamp an event is refused, not built half-wired.
func TestNewSentryReporterRequiresAClock(t *testing.T) {
	t.Parallel()

	if _, err := newSentryReporter(sentryConfig{DSN: "https://abc@host/1"}); err == nil {
		t.Fatal("a reporter without a clock must be refused")
	}
}

// TestSentryReporterSendsTheBoundedReport covers the delivered document: the
// message and the sanitized tags travel, and nothing else does — there is no
// field in which a request payload could ride.
func TestSentryReporterSendsTheBoundedReport(t *testing.T) {
	t.Parallel()

	type captured struct {
		auth string
		body []byte
	}
	delivered := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		delivered <- captured{auth: request.Header.Get("X-Sentry-Auth"), body: body}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reporter, err := newSentryReporter(sentryConfig{
		DSN:         "https://abc123@o1.ingest.sentry.io/42",
		BaseURL:     server.URL,
		Environment: "test",
		Clock:       newTestClock(),
		Logger:      testLogger(),
	})
	if err != nil {
		t.Fatalf("compose the reporter: %v", err)
	}
	defer reporter.Close()

	reporter.Report(ErrorReport{
		Message:   "job handler failed",
		Kind:      "handler",
		Operation: "email_delivery",
		RequestID: "req-1",
	})

	select {
	case request := <-delivered:
		if !strings.Contains(request.auth, "sentry_key=abc123") {
			t.Fatalf("auth header = %q", request.auth)
		}
		lines := strings.Split(strings.TrimRight(string(request.body), "\n"), "\n")
		if len(lines) != 3 {
			t.Fatalf("an envelope is three lines, got %d: %q", len(lines), request.body)
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(lines[2]), &event); err != nil {
			t.Fatalf("the item must be JSON: %v", err)
		}
		allowed := map[string]bool{"event_id": true, "timestamp": true, "platform": true, "level": true, "logger": true, "message": true, "tags": true, "environment": true}
		for key := range event {
			if !allowed[key] {
				t.Fatalf("the event carries an unexpected field %q: %v", key, event)
			}
		}
		if event["message"] != "job handler failed" {
			t.Fatalf("message = %v", event["message"])
		}
		tags, _ := event["tags"].(map[string]any)
		if tags["operation"] != "email_delivery" {
			t.Fatalf("tags = %v", tags)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the report was never delivered")
	}
}

// TestSentryReporterNeverBlocks covers the contract: a full queue drops the
// report and counts the drop instead of parking the caller.
//
// The adapter is built by hand without its posting goroutine, so the queue
// stays exactly full and the count is deterministic rather than racing the
// consumer. Report itself is the production method under test.
func TestSentryReporterNeverBlocks(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	dropped := metrics.Counter("telemetry_errors_dropped_total", "dropped", nil)
	reporter := &sentryReporter{
		queue:   make(chan ErrorReport, 1),
		dropped: dropped,
		logger:  testLogger(),
	}

	reporter.Report(ErrorReport{Message: "fills the queue"})
	done := make(chan struct{})
	go func() {
		for index := 0; index < 100; index++ {
			reporter.Report(ErrorReport{Message: "extra"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Report blocked the caller on a full queue")
	}
	if !strings.Contains(metrics.Render(), "telemetry_errors_dropped_total 100") {
		t.Fatalf("the drops must be counted:\n%s", metrics.Render())
	}
}

// TestNewPostHogSinkAcceptsOnlyAnOrigin covers the host rule: https
// everywhere, plain http only for a loopback fake, and never a path, query,
// fragment or credentials.
func TestNewPostHogSinkAcceptsOnlyAnOrigin(t *testing.T) {
	t.Parallel()

	base := posthogConfig{APIKey: "phc_test", Clock: newTestClock(), Logger: testLogger()}

	accepted := base
	accepted.Host = "https://eu.i.posthog.com"
	sink, err := newPostHogSink(accepted)
	if err != nil {
		t.Fatalf("an https origin must be accepted: %v", err)
	}
	sink.Close()

	loopback := base
	loopback.Host = "http://127.0.0.1:9999"
	sink, err = newPostHogSink(loopback)
	if err != nil {
		t.Fatalf("a loopback fake must be accepted: %v", err)
	}
	sink.Close()

	for _, host := range []string{
		"http://us.i.posthog.com",
		"https://us.i.posthog.com/batch/",
		"https://us.i.posthog.com?key=1",
		"not-a-url",
	} {
		rejected := base
		rejected.Host = host
		if _, err := newPostHogSink(rejected); err == nil {
			t.Fatalf("host %q must be refused", host)
		}
	}
}

// TestNewPostHogSinkRequiresCredential covers the refusal: analytics without a
// project write key is a composition mistake, not a silent no-op adapter.
func TestNewPostHogSinkRequiresCredential(t *testing.T) {
	t.Parallel()

	if _, err := newPostHogSink(posthogConfig{Host: "https://us.i.posthog.com", Clock: newTestClock()}); err == nil {
		t.Fatal("a sink without an API key must be refused")
	}
}

// TestPostHogSinkSendsOnlyAdmittedProperties covers the delivered batch: the
// event carries its scrubbed properties and the account as distinct_id, and
// no request-derived string has a shape to ride in.
func TestPostHogSinkSendsOnlyAdmittedProperties(t *testing.T) {
	t.Parallel()

	type captured struct {
		apiKey string
		batch  []map[string]any
	}
	delivered := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var document struct {
			APIKey string           `json:"api_key"`
			Batch  []map[string]any `json:"batch"`
		}
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &document)
		delivered <- captured{apiKey: document.APIKey, batch: document.Batch}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink, err := newPostHogSink(posthogConfig{
		APIKey:        "phc_test",
		Host:          server.URL,
		Clock:         newTestClock(),
		Logger:        testLogger(),
		FlushInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("compose the sink: %v", err)
	}
	defer sink.Close()

	sink.Capture(Event{
		Name:       EventArenaInfluenceAssigned,
		AccountID:  "acc-1",
		Properties: map[string]any{"locale": "pt-BR", "attributed_count": int64(2)},
	})

	select {
	case request := <-delivered:
		if request.apiKey != "phc_test" {
			t.Fatalf("api_key = %q", request.apiKey)
		}
		if len(request.batch) != 1 {
			t.Fatalf("batch size = %d, want 1", len(request.batch))
		}
		properties, _ := request.batch[0]["properties"].(map[string]any)
		if properties["distinct_id"] != "acc-1" {
			t.Fatalf("properties = %v, want the account as distinct_id", properties)
		}
		if properties["locale"] != "pt-BR" {
			t.Fatalf("properties = %v, want the admitted locale", properties)
		}
		if _, present := properties["email"]; present {
			t.Fatalf("an email must never be sent: %v", properties)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the batch was never delivered")
	}
}

// TestPostHogSinkNeverBlocks covers the same contract as the reporter: a full
// queue drops and counts, and Capture returns at once. Like the reporter test,
// the sink is built without its batching goroutine so the count is exact.
func TestPostHogSinkNeverBlocks(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(newTestClock())
	dropped := metrics.Counter("telemetry_events_dropped_total", "dropped", nil)
	sink := &postHogSink{
		clock:   newTestClock(),
		logger:  testLogger(),
		dropped: dropped,
		queue:   make(chan queuedEvent, 1),
	}

	sink.Capture(Event{Name: EventAccountSignedIn, AccountID: "acc-1"})
	done := make(chan struct{})
	go func() {
		for index := 0; index < 100; index++ {
			sink.Capture(Event{Name: EventAccountSignedIn, AccountID: "acc-1"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Capture blocked the caller on a full queue")
	}
	if !strings.Contains(metrics.Render(), "telemetry_events_dropped_total 100") {
		t.Fatalf("the drops must be counted:\n%s", metrics.Render())
	}
}

// TestBoundedKeepsWholeRunes covers the message bound: truncation never splits
// a multi-byte rune, so no tag or message carries half a character.
func TestBoundedKeepsWholeRunes(t *testing.T) {
	t.Parallel()

	value := strings.Repeat("é", 10)
	cut := bounded(value, 5)
	if len(cut) > 5 {
		t.Fatalf("bounded returned %d bytes, want at most 5", len(cut))
	}
	if !strings.HasPrefix(value, cut) {
		t.Fatalf("bounded returned %q, not a prefix", cut)
	}
	for _, character := range cut {
		if character == '\uFFFD' {
			t.Fatal("truncation split a multi-byte rune")
		}
	}
	if bounded("short", 100) != "short" {
		t.Fatal("a value under the limit must be returned unchanged")
	}
}
