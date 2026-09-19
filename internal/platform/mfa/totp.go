package mfa

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash"
	"time"
)

// HOTP implements the RFC 4226 truncation: HMAC over the counter, dynamic
// truncation, modulo of the requested number of digits.
//
// It is exported because the RFC publishes the counters and the codes they must
// produce (Appendix D), and a test that runs those vectors is the proof that
// this implementation is the algorithm and not an approximation of it.
func HOTP(secret []byte, counter uint64, digits int, algorithm Algorithm) (string, error) {
	if len(secret) == 0 {
		return "", ErrInvalidSecret
	}
	if digits < MinDigits || digits > MaxDigits {
		return "", fmt.Errorf("%w: digits must be between %d and %d", ErrInvalidConfig, MinDigits, MaxDigits)
	}

	mac, err := newHMAC(algorithm, secret)
	if err != nil {
		return "", err
	}

	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	mac.Write(counterBytes[:])
	sum := mac.Sum(nil)

	// Dynamic truncation: the low nibble of the last byte selects the offset,
	// and the sign bit is masked off so the value is always positive.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	modulus := uint32(1)
	for index := 0; index < digits; index++ {
		modulus *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%modulus), nil
}

// newHMAC builds the keyed hash of an algorithm.
func newHMAC(algorithm Algorithm, secret []byte) (hash.Hash, error) {
	switch algorithm {
	case AlgorithmSHA1, "":
		return hmac.New(sha1.New, secret), nil
	case AlgorithmSHA256:
		return hmac.New(sha256.New, secret), nil
	case AlgorithmSHA512:
		return hmac.New(sha512.New, secret), nil
	default:
		return nil, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidConfig, algorithm)
	}
}

// Step is the RFC's time counter: how many periods have elapsed since the Unix
// epoch, in UTC. It is UTC on purpose and by the RFC: a code that depended on
// the server's local zone would break every DST transition.
func (config Config) Step(instant time.Time) (int64, error) {
	normalized, err := config.normalized()
	if err != nil {
		return 0, err
	}
	return normalized.step(instant), nil
}

// step is Step once the configuration is known to be normalized.
func (config Config) step(instant time.Time) int64 {
	return instant.UTC().Unix() / int64(config.Period.Seconds())
}

// Code is the code an authenticator shows at an instant.
func (config Config) Code(secret []byte, instant time.Time) (string, error) {
	normalized, err := config.normalized()
	if err != nil {
		return "", err
	}
	step := normalized.step(instant)
	if step < 0 {
		return "", fmt.Errorf("%w: instant is before the epoch", ErrInvalidConfig)
	}
	return HOTP(secret, uint64(step), normalized.Digits, normalized.Algorithm)
}

// Verify checks one presented code against a secret.
//
// Two rules beyond the RFC live here, and both are the reason this package
// exists instead of a thin wrapper around a library call:
//
//   - the accepted step is searched only within the configured skew. A code
//     from a step outside the window is refused even if it is mathematically
//     correct, because accepting it would mean accepting a code from the past
//     for as long as it takes to brute-force;
//   - a step at or before lastAcceptedStep is refused as a replay. The caller
//     persists that step, which is what makes a code single-use in time: a
//     code that was already accepted — by a shoulder-surfer who read it, or by
//     a replay of a captured request — cannot be accepted twice, even inside
//     the same window.
//
// The comparison is constant-time, and the search does not stop early on a
// non-match in a way that depends on the code's contents.
func (config Config) Verify(secret []byte, code string, instant time.Time, lastAcceptedStep int64) (int64, error) {
	normalized, err := config.normalized()
	if err != nil {
		return 0, err
	}
	if len(secret) == 0 {
		return 0, ErrInvalidSecret
	}

	presented := normalizeCode(code)
	if len(presented) != normalized.Digits || !isDigits(presented) {
		return 0, ErrInvalidCode
	}

	current := normalized.step(instant)
	if current < 0 {
		return 0, fmt.Errorf("%w: instant is before the epoch", ErrInvalidConfig)
	}

	matched := int64(-1)
	// The window is walked in a fixed order and every step is computed, so the
	// cost of a verification does not reveal which step matched.
	for offset := -normalized.Skew; offset <= normalized.Skew; offset++ {
		step := current + int64(offset)
		if step < 0 {
			continue
		}
		candidate, err := HOTP(secret, uint64(step), normalized.Digits, normalized.Algorithm)
		if err != nil {
			return 0, err
		}
		if equalInConstantTime(candidate, presented) && matched < 0 {
			matched = step
		}
	}

	if matched < 0 {
		return 0, ErrInvalidCode
	}
	if matched <= lastAcceptedStep {
		return 0, ErrReplayedStep
	}
	return matched, nil
}

// normalizeCode strips the separators a person or an authenticator may add. A
// code is digits; space and dash are formatting, never content.
func normalizeCode(code string) string {
	var builder []byte
	for index := 0; index < len(code); index++ {
		switch character := code[index]; character {
		case ' ', '-', '\t':
			continue
		default:
			builder = append(builder, character)
		}
	}
	return string(builder)
}

// isDigits reports whether every byte is an ASCII digit.
func isDigits(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}
