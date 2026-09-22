package observability

import (
	"log/slog"
	"sync/atomic"
)

// analytics is the front of the analytics path every surface receives. It
// validates, samples and hands the event to each sink's bounded queue, and it
// never blocks: Scrubbed is a bounded map walk over call-site-owned values,
// the sampling hash is arithmetic, and every sink answers immediately by
// contract.
type analytics struct {
	sinks   []EventSink
	sampler *Sampler
	logger  *slog.Logger

	received   *Counter
	refused    *Counter
	sampledOut *Counter
	handed     *Counter

	refusalLogged atomic.Bool
}

// analyticsConfig gathers what the front needs. Metrics is required.
type analyticsConfig struct {
	Sinks   []EventSink
	Sampler *Sampler
	Logger  *slog.Logger
	Metrics *Metrics
}

// newAnalytics builds the front. A process composed without a provider still
// gets a live front: the allowlist refusal and the sampling decision are the
// same in every environment, only the sinks differ.
func newAnalytics(config analyticsConfig) *analytics {
	return &analytics{
		sinks:      config.Sinks,
		sampler:    config.Sampler,
		logger:     config.Logger,
		received:   config.Metrics.Counter("telemetry_events_received_total", "Analytics events the process attempted to record.", nil),
		refused:    config.Metrics.Counter("telemetry_events_refused_total", "Analytics events refused by the allowlist (a programming error, not configuration).", nil),
		sampledOut: config.Metrics.Counter("telemetry_events_sampled_out_total", "Analytics events dropped by the configured sampling rate.", nil),
		handed:     config.Metrics.Counter("telemetry_events_handed_total", "Analytics events handed to every configured sink.", nil),
	}
}

// Capture implements EventSink. It never blocks and never panics.
func (front *analytics) Capture(event Event) {
	if front == nil {
		return
	}
	front.received.Inc()

	properties, err := Scrubbed(event)
	if err != nil {
		front.refused.Inc()
		// One line per process: a call site that got the allowlist wrong
		// would otherwise turn a burst into a log flood.
		if front.logger != nil && front.refusalLogged.CompareAndSwap(false, true) {
			front.logger.Error(
				"observability: analytics event refused (this is a programming error, not configuration)",
				slog.String("event", event.Name),
				slog.String("reason", err.Error()),
			)
		}
		return
	}

	key := event.RequestID
	if key == "" {
		key = event.AccountID
	}
	if !front.sampler.Keep(key) {
		front.sampledOut.Inc()
		return
	}

	delivered := event
	delivered.Properties = properties
	for _, sink := range front.sinks {
		sink.Capture(delivered)
	}
	front.handed.Inc()
}
