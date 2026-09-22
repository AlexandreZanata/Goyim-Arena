package config

import (
	"strings"
	"testing"
)

// observabilityEnv is the environment of every observability case: the
// development baseline plus the entries of the case.
func observabilityEnv(entries ...string) []string {
	return environ(entries...)
}

// TestObservabilityConfigurationDefaultsToNothing covers the safe default:
// without the telemetry variables the process still boots, no provider is
// configured and analytics is unsampled at 100%.
func TestObservabilityConfigurationDefaultsToNothing(t *testing.T) {
	t.Parallel()

	config, err := Load(observabilityEnv())
	if err != nil {
		t.Fatalf("load without observability configuration: %v", err)
	}
	if config.SentryDSN().IsSet() {
		t.Fatal("no error reporter credential must be configured by default")
	}
	if config.PostHogAPIKey().IsSet() {
		t.Fatal("no analytics credential must be configured by default")
	}
	if config.PostHogHost() != "" {
		t.Fatalf("analytics host = %q, want the provider default", config.PostHogHost())
	}
	if config.AnalyticsSampleRate() != DefaultAnalyticsSampleRate {
		t.Fatalf("sample rate = %d, want %d", config.AnalyticsSampleRate(), DefaultAnalyticsSampleRate)
	}
}

// TestObservabilityConfigurationAcceptsGoodValues covers the accepted shapes
// and that a credential is handed over exactly, never printed.
func TestObservabilityConfigurationAcceptsGoodValues(t *testing.T) {
	t.Parallel()

	config, err := Load(observabilityEnv(
		SentryDSNVariable+"=https://abc123@o1.ingest.sentry.io/42",
		PostHogAPIKeyVariable+"=phc_abcdef",
		PostHogHostVariable+"=https://eu.i.posthog.com",
		AnalyticsSampleRateVariable+"=25",
	))
	if err != nil {
		t.Fatalf("load with telemetry: %v", err)
	}
	if string(config.SentryDSN().Unredacted()) != "https://abc123@o1.ingest.sentry.io/42" {
		t.Fatal("the configured DSN must be handed over exactly")
	}
	if string(config.PostHogAPIKey().Unredacted()) != "phc_abcdef" {
		t.Fatal("the configured key must be handed over exactly")
	}
	if config.PostHogHost() != "https://eu.i.posthog.com" {
		t.Fatalf("host = %q", config.PostHogHost())
	}
	if config.AnalyticsSampleRate() != 25 {
		t.Fatalf("sample rate = %d, want 25", config.AnalyticsSampleRate())
	}
}

// TestObservabilityCredentialsRedactThemselves covers the redaction rule: a
// value that must never be logged does not render itself.
func TestObservabilityCredentialsRedactThemselves(t *testing.T) {
	t.Parallel()

	config, err := Load(observabilityEnv(
		SentryDSNVariable+"=https://abc123@o1.ingest.sentry.io/42",
		PostHogAPIKeyVariable+"=phc_abcdef",
	))
	if err != nil {
		t.Fatalf("load with telemetry: %v", err)
	}
	if rendered := config.SentryDSN().String(); strings.Contains(rendered, "abc123") {
		t.Fatalf("the DSN printed its credential: %q", rendered)
	}
	if rendered := config.PostHogAPIKey().String(); strings.Contains(rendered, "abcdef") {
		t.Fatalf("the key printed itself: %q", rendered)
	}
}

// TestObservabilityConfigurationRefusesUnusableValues covers every rejected
// shape, each naming its variable and never echoing the value.
func TestObservabilityConfigurationRefusesUnusableValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		entry    string
		variable string
	}{
		{"dsn without https", SentryDSNVariable + "=abc@host/1", SentryDSNVariable},
		{"dsn without key", SentryDSNVariable + "=https://host/1", SentryDSNVariable},
		{"dsn padded", SentryDSNVariable + "= https://abc@host/1", SentryDSNVariable},
		{"key without prefix", PostHogAPIKeyVariable + "=abcdef", PostHogAPIKeyVariable},
		{"key padded", PostHogAPIKeyVariable + "= phc_abcdef", PostHogAPIKeyVariable},
		{"host without scheme", PostHogHostVariable + "=us.i.posthog.com", PostHogHostVariable},
		{"host on plain http", PostHogHostVariable + "=http://us.i.posthog.com", PostHogHostVariable},
		{"host with a path", PostHogHostVariable + "=https://us.i.posthog.com/batch/", PostHogHostVariable},
		{"sample rate text", AnalyticsSampleRateVariable + "=half", AnalyticsSampleRateVariable},
		{"sample rate high", AnalyticsSampleRateVariable + "=101", AnalyticsSampleRateVariable},
		{"sample rate negative", AnalyticsSampleRateVariable + "=-1", AnalyticsSampleRateVariable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(observabilityEnv(testCase.entry))
			if err == nil {
				t.Fatalf("%s must be refused", testCase.entry)
			}
			if !strings.Contains(err.Error(), testCase.variable) {
				t.Fatalf("error %q must name %s", err, testCase.variable)
			}
		})
	}
}

// TestObservabilityConfigurationAcceptsALoopbackFakeHost covers the test-only
// exception: plain http is accepted for a loopback host, so a local fake can
// receive the batch.
func TestObservabilityConfigurationAcceptsALoopbackFakeHost(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		config, err := Load(observabilityEnv(PostHogHostVariable + "=" + host))
		if err != nil {
			t.Fatalf("load with loopback host %q: %v", host, err)
		}
		if config.PostHogHost() != host {
			t.Fatalf("host = %q, want %q", config.PostHogHost(), host)
		}
	}
}

// TestObservabilityConfigurationIsOptionalInProduction covers the boot rule:
// telemetry is not required in production, and its absence never refuses the
// boot the way a missing payment or email credential does.
func TestObservabilityConfigurationIsOptionalInProduction(t *testing.T) {
	t.Parallel()

	if _, err := Load(productionEnv()); err != nil {
		t.Fatalf("production without telemetry must boot: %v", err)
	}
}
