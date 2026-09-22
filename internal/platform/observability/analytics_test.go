package observability

import (
	"strings"
	"testing"
)

// newTestAnalytics builds the front over the given sinks with every event
// kept, which is the composition without credentials.
func newTestAnalytics(sinks ...EventSink) (*analytics, *Metrics) {
	metrics := NewMetrics(newTestClock())
	front := newAnalytics(analyticsConfig{
		Sinks:   sinks,
		Sampler: NewSampler(100, 100),
		Logger:  testLogger(),
		Metrics: metrics,
	})
	return front, metrics
}

// TestAnalyticsHandsScrubbedEventsToEverySink covers the happy path: the sink
// receives the scrubbed document, never the raw call-site map.
func TestAnalyticsHandsScrubbedEventsToEverySink(t *testing.T) {
	t.Parallel()

	first := &recordingSink{}
	second := &recordingSink{}
	front, _ := newTestAnalytics(first, second)

	front.Capture(Event{
		Name:       EventArenaInfluenceAssigned,
		AccountID:  "acc-1",
		RequestID:  "req-1",
		Properties: map[string]any{"locale": "pt-BR", "attributed_count": 2},
	})

	for index, sink := range []*recordingSink{first, second} {
		events := sink.captured()
		if len(events) != 1 {
			t.Fatalf("sink %d received %d events, want 1", index, len(events))
		}
		if events[0].Properties["locale"] != "pt-BR" {
			t.Fatalf("sink %d received %v, want the scrubbed locale", index, events[0].Properties)
		}
	}
}

// TestAnalyticsRefusesEventOutsideTheAllowlist covers the refusal: a
// programming error reaches no sink and is counted, so the mistake is visible
// in the metrics instead of silently recorded.
func TestAnalyticsRefusesEventOutsideTheAllowlist(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	front, metrics := newTestAnalytics(sink)

	front.Capture(Event{Name: "account.not_a_real_event", AccountID: "acc-1"})

	if len(sink.captured()) != 0 {
		t.Fatal("a refused event must reach no sink")
	}
	if !strings.Contains(metrics.Render(), "telemetry_events_refused_total 1") {
		t.Fatalf("the refusal must be counted:\n%s", metrics.Render())
	}
}

// TestAnalyticsDropsSampledOutEvents covers sampling: an event outside the
// kept fraction reaches no sink and is counted as sampled out, never confused
// with a refusal.
func TestAnalyticsDropsSampledOutEvents(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	metrics := NewMetrics(newTestClock())
	front := newAnalytics(analyticsConfig{
		Sinks:   []EventSink{sink},
		Sampler: NewSampler(0, 100),
		Logger:  testLogger(),
		Metrics: metrics,
	})

	front.Capture(Event{Name: EventAccountSignedIn, AccountID: "acc-1", RequestID: "req-1"})

	if len(sink.captured()) != 0 {
		t.Fatal("a sampled-out event must reach no sink")
	}
	rendered := metrics.Render()
	if !strings.Contains(rendered, "telemetry_events_sampled_out_total 1") {
		t.Fatalf("the sampling drop must be counted:\n%s", rendered)
	}
	if strings.Contains(rendered, "telemetry_events_refused_total 1") {
		t.Fatal("a sampling drop is not a refusal")
	}
}

// TestAnalyticsSamplingKeyIsStablePerRequest covers the decision key: two
// events of the same request share the decision, which is what makes a traced
// percent coherent.
func TestAnalyticsSamplingKeyIsStablePerRequest(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	metrics := NewMetrics(newTestClock())
	front := newAnalytics(analyticsConfig{
		Sinks:   []EventSink{sink},
		Sampler: NewSampler(50, 100),
		Logger:  testLogger(),
		Metrics: metrics,
	})

	for index := 0; index < 20; index++ {
		front.Capture(Event{Name: EventAccountSignedIn, AccountID: "acc-1", RequestID: "req-fixed"})
	}
	received := len(sink.captured())
	if received != 0 && received != 20 {
		t.Fatalf("received %d of 20, want all or none for one request id", received)
	}
}
