package domain

import "time"

// MethodologyVersion pins the derivation rules behind every published
// snapshot (docs/TRANSPARENCY.md §3/§6). A new version means a conscious
// edit here with its tests: methodology changes must declare whether
// history was recalculated, and P14-T03 publishes this version alongside
// every document.
const MethodologyVersion = 1

// LowCountThreshold bounds reidentification from small cells
// (docs/TRANSPARENCY.md §2/§3): metric values below the threshold report
// zero instead of the exact count. The threshold applies uniformly,
// including to large-platform totals, so small deployments never leak
// precise micro-populations through the same document shape.
const LowCountThreshold = 5

// Period is the half-open UTC interval a snapshot covers: [Start, End).
type Period struct {
	start time.Time
	end   time.Time
}

// NewPeriod builds a period with the end strictly after the start. Both
// instants normalize to UTC.
func NewPeriod(start, end time.Time) (Period, error) {
	if start.IsZero() || end.IsZero() {
		return Period{}, ErrInvalidPeriod
	}
	startUTC := start.UTC()
	endUTC := end.UTC()
	if !endUTC.After(startUTC) {
		return Period{}, ErrInvalidPeriod
	}
	return Period{start: startUTC, end: endUTC}, nil
}

// Start returns the inclusive start of the period in UTC.
func (p Period) Start() time.Time { return p.start }

// End returns the exclusive end of the period in UTC.
func (p Period) End() time.Time { return p.end }

// IsZero reports whether the period is uninitialized.
func (p Period) IsZero() bool { return p.start.IsZero() || p.end.IsZero() }

// Suppress applies the low-count rule: values below the threshold report
// zero. Negative inputs are incoherent and report zero as well, so a
// miscount can never surface as a negative public metric.
func Suppress(value int64) int64 {
	if value < LowCountThreshold {
		return 0
	}
	return value
}
