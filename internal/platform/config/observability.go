package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Observability configuration (P19-T05).
//
// Telemetry is optional where the payment and email credentials are not: a
// process without them serves the product and records nothing, and the
// provider adapters are simply not composed. What is not optional is the
// shape of a value that is set — a malformed DSN or host is refused at boot,
// naming the variable, instead of failing as a dropped event nobody watches.
//
//	ARENA_SENTRY_DSN=https://<key>@<host>/<project>
//	ARENA_POSTHOG_API_KEY=phc_...
//	ARENA_POSTHOG_HOST=https://us.i.posthog.com
//	ARENA_ANALYTICS_SAMPLE_RATE=100
//
// The two credentials are secrets and redact themselves; the host and the
// sample rate are plain configuration.
const (
	// SentryDSNVariable carries the error reporter credential.
	SentryDSNVariable = "ARENA_SENTRY_DSN"

	// PostHogAPIKeyVariable carries the product analytics project write key.
	PostHogAPIKeyVariable = "ARENA_POSTHOG_API_KEY"

	// PostHogHostVariable overrides the analytics API origin. It exists for
	// the EU cloud and for a local fake in tests; the default is the US
	// cloud.
	PostHogHostVariable = "ARENA_POSTHOG_HOST"

	// AnalyticsSampleRateVariable is the deterministic sampling percentage of
	// analytics, 0..100. Errors are never sampled.
	AnalyticsSampleRateVariable = "ARENA_ANALYTICS_SAMPLE_RATE"
)

const (
	// maxObservabilityCredentialLength bounds both credentials defensively,
	// so a misfiled blob is refused instead of being handed to a provider.
	maxObservabilityCredentialLength = 512

	// postHogAPIKeyPrefix is the shape the provider issues to a project. It
	// catches the classic misconfiguration of pasting a personal key or a
	// value copied from another console.
	postHogAPIKeyPrefix = "phc_"

	// DefaultAnalyticsSampleRate records every allowlisted event.
	DefaultAnalyticsSampleRate = 100
)

// parseSentryDSN validates the shape of the provider DSN without echoing it.
// The adapter parses it fully at composition; this edge catches the forms an
// operator gets wrong at boot, with the variable named.
func parseSentryDSN(raw string) (Secret, ValidationErrors) {
	secret := NewSecret(raw)
	if raw == "" {
		return Secret{}, nil
	}

	problem := ""
	switch {
	case len(raw) > maxObservabilityCredentialLength:
		problem = fmt.Sprintf("is longer than %d characters (is this really the provider DSN?)", maxObservabilityCredentialLength)
	case strings.TrimSpace(raw) != raw:
		problem = "contains surrounding whitespace (copy the value without spaces or quotes)"
	case !isPrintableASCII(raw):
		problem = "contains characters outside printable ASCII"
	case !strings.HasPrefix(raw, "https://"):
		problem = "must be an https://<key>@<host>/<project> DSN"
	case !strings.Contains(raw, "@"):
		problem = "is missing the public key before @"
	}
	if problem != "" {
		return Secret{}, ValidationErrors{{Variable: SentryDSNVariable, Problem: problem}}
	}
	return secret, nil
}

// parsePostHogAPIKey validates the analytics credential shape without ever
// echoing the value.
func parsePostHogAPIKey(raw string) (Secret, ValidationErrors) {
	secret := NewSecret(raw)
	if raw == "" {
		return Secret{}, nil
	}

	problem := ""
	body, hasPrefix := strings.CutPrefix(raw, postHogAPIKeyPrefix)
	switch {
	case len(raw) > maxObservabilityCredentialLength:
		problem = fmt.Sprintf("is longer than %d characters (is this really the project key?)", maxObservabilityCredentialLength)
	case strings.TrimSpace(raw) != raw:
		problem = "contains surrounding whitespace (copy the value without spaces or quotes)"
	case !isPrintableASCII(raw):
		problem = "contains characters outside printable ASCII"
	case !hasPrefix || body == "":
		problem = fmt.Sprintf("must be a project write key (%s...)", postHogAPIKeyPrefix)
	}
	if problem != "" {
		return Secret{}, ValidationErrors{{Variable: PostHogAPIKeyVariable, Problem: problem}}
	}
	return secret, nil
}

// parsePostHogHost validates the optional API origin: https everywhere, plain
// http only for a loopback host (the local fake), and no path, query,
// fragment or credentials — the adapter appends its own batch path.
func parsePostHogHost(raw string) (string, ValidationErrors) {
	if raw == "" {
		return "", nil
	}

	problem := ""
	parsed, err := url.Parse(raw)
	switch {
	case err != nil || parsed.Scheme == "" || parsed.Host == "":
		problem = "is not an absolute URL (expected, for example, https://us.i.posthog.com)"
	case parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHostname(parsed.Hostname())):
		problem = "must use https (plain http is accepted only for a loopback host)"
	case parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/"):
		problem = "must be an origin without path, query, fragment or credentials"
	}
	if problem != "" {
		return "", ValidationErrors{{Variable: PostHogHostVariable, Problem: problem}}
	}
	return raw, nil
}

// parseAnalyticsSampleRate validates the sampling percentage.
func parseAnalyticsSampleRate(raw string) (int, ValidationErrors) {
	rate, err := strconv.Atoi(raw)
	if err != nil || rate < 0 || rate > 100 {
		return 0, ValidationErrors{{
			Variable: AnalyticsSampleRateVariable,
			Problem:  fmt.Sprintf("invalid value %q (must be an integer between 0 and 100)", raw),
		}}
	}
	return rate, nil
}

// isLoopbackHostname reports whether the host is a loopback literal.
func isLoopbackHostname(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

// SentryDSN returns the redacted error reporter credential. The composition
// root passes it to the adapter through Unredacted and nothing else reads it.
func (config Config) SentryDSN() Secret { return config.sentryDSN }

// PostHogAPIKey returns the redacted product analytics write key.
func (config Config) PostHogAPIKey() Secret { return config.posthogAPIKey }

// PostHogHost returns the analytics API origin, and the empty string when the
// provider default applies.
func (config Config) PostHogHost() string { return config.posthogHost }

// AnalyticsSampleRate returns the deterministic analytics sampling
// percentage, 0..100.
func (config Config) AnalyticsSampleRate() int { return config.analyticsSampleRate }
