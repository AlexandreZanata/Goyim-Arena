package config

import (
	"fmt"
	"strings"
	"time"
)

// Stripe adapter configuration (P12-T03).
//
// The payment adapter needs exactly two things from the environment: the
// credential it authenticates with and the deadline that bounds every provider
// call.
//
//	ARENA_STRIPE_SECRET_KEY=sk_test_...
//	ARENA_STRIPE_TIMEOUT=10s
//
// Idempotency keys are deliberately NOT configured here: a key must be stable
// across the retries of one operation, so it belongs to the use case that owns
// the operation (the local checkout intent) and never to the deployment.
//
// The endpoint is not configurable either: a deployment can never be pointed at
// another host, so the credential can only ever be sent to the provider.
const (
	// stripeSecretKeyVariable carries the provider API secret.
	stripeSecretKeyVariable = "ARENA_STRIPE_SECRET_KEY"

	// stripeTimeoutVariable bounds one provider call.
	stripeTimeoutVariable = "ARENA_STRIPE_TIMEOUT"

	// maxStripeSecretKeyLength bounds the credential defensively, so a
	// misfiled blob is refused instead of being sent to the provider.
	maxStripeSecretKeyLength = 256

	// maxStripeTimeout bounds a provider call: a wider deadline would hold a
	// server goroutine and a request context open with no operational gain.
	maxStripeTimeout = 60 * time.Second
)

// stripeSecretPrefixes are the credential prefixes the provider issues for
// server-side keys: secret keys and restricted keys. Requiring one catches the
// classic misconfiguration of pasting a publishable key (pk_...) into a secret
// slot, which would otherwise only surface on the first purchase.
var stripeSecretPrefixes = []string{"sk_", "rk_"}

// parseStripeSecretKey validates the credential shape without ever echoing the
// value: a problem message names the variable and the reason, never the secret.
func parseStripeSecretKey(raw string) (Secret, ValidationErrors) {
	secret := NewSecret(raw)
	if raw == "" {
		return Secret{}, nil
	}

	problem := ""
	switch {
	case len(raw) > maxStripeSecretKeyLength:
		problem = fmt.Sprintf("is longer than %d characters (is this really the provider secret?)", maxStripeSecretKeyLength)
	case strings.TrimSpace(raw) != raw:
		problem = "contains surrounding whitespace (copy the value without spaces or quotes)"
	case !hasStripeSecretPrefix(raw):
		problem = "must be a server-side Stripe key (sk_... or rk_...), never a publishable key"
	case !isPrintableASCII(raw):
		problem = "contains characters outside printable ASCII"
	}
	if problem != "" {
		return Secret{}, ValidationErrors{{Variable: stripeSecretKeyVariable, Problem: problem}}
	}
	return secret, nil
}

// hasStripeSecretPrefix reports whether the credential carries one of the
// server-side prefixes and an actual key body after it.
func hasStripeSecretPrefix(raw string) bool {
	for _, prefix := range stripeSecretPrefixes {
		if body, found := strings.CutPrefix(raw, prefix); found {
			return body != ""
		}
	}
	return false
}

// isPrintableASCII reports whether every byte is printable ASCII, the character
// set an HTTP header value accepts.
func isPrintableASCII(value string) bool {
	for index := 0; index < len(value); index++ {
		if character := value[index]; character < '!' || character > '~' {
			return false
		}
	}
	return true
}

// parseStripeTimeout validates the deadline of one provider call.
func parseStripeTimeout(raw string) (time.Duration, ValidationErrors) {
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, ValidationErrors{{
			Variable: stripeTimeoutVariable,
			Problem:  fmt.Sprintf("invalid duration %q (must be positive, for example 10s, 30s)", raw),
		}}
	}
	if duration > maxStripeTimeout {
		return 0, ValidationErrors{{
			Variable: stripeTimeoutVariable,
			Problem:  fmt.Sprintf("must not exceed %s: a provider call may not hold a server goroutine longer than that", maxStripeTimeout),
		}}
	}
	return duration, nil
}

// StripeSecretKey returns the redacted provider credential. The composition
// root passes it to the adapter through Unredacted and nothing else reads it.
func (config Config) StripeSecretKey() Secret { return config.stripeSecretKey }

// StripeTimeout returns the deadline applied to every provider call.
func (config Config) StripeTimeout() time.Duration { return config.stripeTimeout }
