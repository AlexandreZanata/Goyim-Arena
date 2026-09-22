package observability

import (
	"hash/maphash"
)

// Sampler is the deterministic per-key decision of whether one unit of
// telemetry is recorded. It exists for one reason: at production volume
// analytics is sampled, and the decision has to be stable — the same request
// is always in or always out — so a recorded percent is traceable instead of
// a random percent of events.
type Sampler struct {
	// numerator/denominator is the kept fraction, as integer arithmetic: no
	// float, no library.
	numerator   uint64
	denominator uint64
	seed        maphash.Seed
}

// NewSampler builds a sampler that keeps numerator out of denominator units
// (1/1 keeps everything, 0/1 keeps nothing). An impossible fraction selects
// 1/1 instead of failing: sampling is an operational knob, not a policy.
func NewSampler(numerator, denominator uint64) *Sampler {
	if denominator == 0 || numerator > denominator {
		numerator, denominator = 1, 1
	}
	return &Sampler{numerator: numerator, denominator: denominator, seed: maphash.MakeSeed()}
}

// Keep reports whether the key is sampled in. The hash is stable for the
// lifetime of the process: the same key is either always recorded or never
// recorded, which is the property that keeps one traced percent coherent.
func (sampler *Sampler) Keep(key string) bool {
	if sampler == nil {
		return true
	}
	if sampler.numerator == 0 {
		return false
	}
	if sampler.numerator >= sampler.denominator {
		return true
	}
	return maphash.String(sampler.seed, key)%sampler.denominator < sampler.numerator
}
