package observability

import "testing"

// TestSamplerEdges covers the two ends: keeping everything and keeping
// nothing, which is how a process turns analytics off without disabling the
// front.
func TestSamplerEdges(t *testing.T) {
	t.Parallel()

	all := NewSampler(100, 100)
	none := NewSampler(0, 100)
	for index := 0; index < 50; index++ {
		key := string(rune('a' + index%26))
		if !all.Keep(key) {
			t.Fatalf("100/100 must keep %q", key)
		}
		if none.Keep(key) {
			t.Fatalf("0/100 must drop %q", key)
		}
	}
}

// TestSamplerIsStablePerKey covers the property the sampler exists for: the
// same key is always in or always out, so a recorded percent is traceable
// instead of a fresh coin flip on every event.
func TestSamplerIsStablePerKey(t *testing.T) {
	t.Parallel()

	sampler := NewSampler(50, 100)
	decision := sampler.Keep("req-abc")
	for index := 0; index < 100; index++ {
		if sampler.Keep("req-abc") != decision {
			t.Fatal("the sampling decision must be stable for one key")
		}
	}
}

// TestSamplerRefusesImpossibleFraction covers the operational knob: a fraction
// that cannot mean anything selects 1/1 rather than panicking or dropping
// everything.
func TestSamplerRefusesImpossibleFraction(t *testing.T) {
	t.Parallel()

	for _, sampler := range []*Sampler{NewSampler(2, 1), NewSampler(1, 0)} {
		if !sampler.Keep("any") {
			t.Fatal("an impossible fraction must select 1/1")
		}
	}
	// A nil sampler keeps everything, so a zero value is a working front.
	var nilSampler *Sampler
	if !nilSampler.Keep("any") {
		t.Fatal("a nil sampler must keep everything")
	}
}

// TestSamplerKeepsApproximatelyTheConfiguredFraction covers the middle: over
// many distinct keys the kept fraction is close to the configured rate, which
// is the only claim a percentage can make.
func TestSamplerKeepsApproximatelyTheConfiguredFraction(t *testing.T) {
	t.Parallel()

	sampler := NewSampler(50, 100)
	kept := 0
	const total = 20000
	for index := 0; index < total; index++ {
		if sampler.Keep("request-" + itoa(index)) {
			kept++
		}
	}
	ratio := float64(kept) / float64(total)
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("kept fraction = %.3f, want about 0.50", ratio)
	}
}

// itoa is the small integer renderer the sampling test needs without pulling
// strconv into the assertion.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
