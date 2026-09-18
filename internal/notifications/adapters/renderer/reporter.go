package renderer

import "log/slog"

// LogFallbackReporter records every localization fallback on the application
// log, which is where an operator alert is built from.
//
// The event it receives names a template, a locale and a key: a recipient, a
// display name or a one-time code has no field to travel in, so this metric
// cannot carry personal data. That is the whole reason the fallback takes a
// closed event instead of a format string and arguments — the log line is safe
// by construction rather than by a reviewer remembering to redact it.
//
// A nil logger falls back to the process default instead of discarding: a
// fallback that nobody sees is exactly the silent degradation the standard
// forbids.
type LogFallbackReporter struct {
	logger *slog.Logger
}

var _ FallbackReporter = (*LogFallbackReporter)(nil)

// NewLogFallbackReporter wires the reporter.
func NewLogFallbackReporter(logger *slog.Logger) *LogFallbackReporter {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogFallbackReporter{logger: logger}
}

// ReportFallback records one fallback at warning level: it is a defect — a
// shipped locale is missing a string — not an expected state.
func (r *LogFallbackReporter) ReportFallback(event FallbackEvent) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Warn("email rendered from the fallback locale",
		slog.String("template", event.Template.String()),
		slog.String("requested_locale", event.Requested.String()),
		slog.String("fallback_locale", event.Source.String()),
		slog.String("message_key", event.Key),
	)
}
