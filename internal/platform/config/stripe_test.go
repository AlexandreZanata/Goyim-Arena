package config

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestStripeConfigurationDefaultsToNothing covers the safe default: without the
// billing variables the process still boots and nothing can reach a provider.
func TestStripeConfigurationDefaultsToNothing(t *testing.T) {
	t.Parallel()

	config, err := Load(environ())
	if err != nil {
		t.Fatalf("load without provider configuration: %v", err)
	}
	if config.StripeSecretKey().IsSet() {
		t.Fatal("no provider credential must be configured by default")
	}
	if config.StripeTimeout() != 10*time.Second {
		t.Fatalf("default provider timeout = %s, want 10s", config.StripeTimeout())
	}
}

// TestStripeConfigurationAcceptsServerSideKeys covers the accepted credential
// shapes and the timeout bounds.
func TestStripeConfigurationAcceptsServerSideKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"sk_test_abc123", "sk_live_abc123", "rk_live_restricted"} {
		config, err := Load(environ("ARENA_STRIPE_SECRET_KEY=" + key))
		if err != nil {
			t.Fatalf("load with %q: %v", key, err)
		}
		if string(config.StripeSecretKey().Unredacted()) != key {
			t.Fatal("the configured credential must be handed over exactly")
		}
	}

	for _, raw := range []string{"1s", "10s", "45s", "1m"} {
		config, err := Load(environ("ARENA_STRIPE_TIMEOUT=" + raw))
		if err != nil {
			t.Fatalf("load with timeout %q: %v", raw, err)
		}
		want, err := time.ParseDuration(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if config.StripeTimeout() != want {
			t.Fatalf("timeout = %s, want %s", config.StripeTimeout(), want)
		}
	}
}

// TestStripeConfigurationRefusesUnusableValues covers every rejected shape: the
// classic mistaken publishable key, a whitespace-padded paste, a secret that is
// not printable and a deadline outside the operational range.
func TestStripeConfigurationRefusesUnusableValues(t *testing.T) {
	t.Parallel()

	credentialCases := []struct {
		name string
		raw  string
	}{
		{name: "publishable key", raw: "pk_live_publishable"},
		{name: "missing prefix", raw: "abc123"},
		{name: "empty body", raw: "sk_"},
		{name: "surrounding whitespace", raw: " sk_test_abc123"},
		{name: "trailing newline", raw: "sk_test_abc123\n"},
		{name: "too long", raw: "sk_" + strings.Repeat("a", 254)},
		{name: "non ascii", raw: "sk_test_ação"},
	}
	for _, testCase := range credentialCases {
		t.Run("credential/"+testCase.name, func(t *testing.T) {
			t.Parallel()

			config, err := Load(environ("ARENA_STRIPE_SECRET_KEY=" + testCase.raw))
			if err == nil {
				t.Fatalf("value %q must be refused", testCase.raw)
			}
			if config.StripeSecretKey().IsSet() {
				t.Fatal("a refused credential must never be stored")
			}
			if !strings.Contains(err.Error(), "ARENA_STRIPE_SECRET_KEY") {
				t.Errorf("error must name the variable: %v", err)
			}
			// The problem message names the variable and a shape, never the
			// value: the configuration vocabulary never quotes a credential,
			// so a quoted rendering would mean the value traveled.
			if quoted := strconv.Quote(strings.TrimSpace(testCase.raw)); strings.Contains(err.Error(), quoted) {
				t.Errorf("error leaked the credential: %v", err)
			}
		})
	}

	timeoutCases := []string{"", "0s", "-5s", "soon", "61s", "2m"}
	for _, raw := range timeoutCases {
		t.Run("timeout/"+raw, func(t *testing.T) {
			t.Parallel()

			_, err := Load(environ("ARENA_STRIPE_TIMEOUT=" + raw))
			if err == nil {
				t.Fatalf("timeout %q must be refused", raw)
			}
			if !strings.Contains(err.Error(), "ARENA_STRIPE_TIMEOUT") {
				t.Errorf("error must name the variable: %v", err)
			}
		})
	}
}

// TestProductionRequiresTheProviderCredential pins the production safety rule:
// selling is impossible without the credential, so a production deployment
// without one refuses to boot instead of failing on the first purchase.
func TestProductionRequiresTheProviderCredential(t *testing.T) {
	t.Parallel()

	// Everything else in production is satisfied; the credential is the single
	// blank, so the only refusal left to report is the one under test.
	_, err := Load(productionEnv("ARENA_STRIPE_SECRET_KEY="))
	if err == nil {
		t.Fatal("production without a provider credential must fail")
	}
	if !strings.Contains(err.Error(), stripeSecretKeyVariable) ||
		!strings.Contains(err.Error(), "required when ARENA_ENV=production") {
		t.Fatalf("error must name the missing production requirement: %v", err)
	}

	if _, err := Load(environ("ARENA_STRIPE_SECRET_KEY=sk_test_development")); err != nil {
		t.Fatalf("development must accept a test credential: %v", err)
	}
}

// TestStripeVariablesAreKnown keeps a typo from being silently ignored.
func TestStripeVariablesAreKnown(t *testing.T) {
	t.Parallel()

	_, err := Load(environ("ARENA_STRIPE_SECRET_KEEY=sk_test_abc"))
	if err == nil {
		t.Fatal("a typo in a provider variable must fail the load")
	}
	if !strings.Contains(err.Error(), "ARENA_STRIPE_SECRET_KEEY") {
		t.Fatalf("error must name the unknown variable: %v", err)
	}
}

// TestStripeCredentialNeverPrints proves the credential follows the Secret
// redaction rules: the Config and the accessor render redacted while the
// explicit hand-off still exposes the exact value.
func TestStripeCredentialNeverPrints(t *testing.T) {
	t.Parallel()

	const credential = "sk_live_never_print_me"
	config, err := Load(productionEnv("ARENA_STRIPE_SECRET_KEY=" + credential))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for index, rendered := range []string{
		config.String(),
		config.StripeSecretKey().String(),
		renderedConfig(config),
	} {
		if strings.Contains(rendered, credential) {
			t.Errorf("rendering %d leaked the credential: %q", index, rendered)
		}
	}
	if string(config.StripeSecretKey().Unredacted()) != credential {
		t.Fatal("Unredacted must expose the exact credential to the composition root")
	}
}

// renderedConfig renders the whole structure the way a debugger would.
func renderedConfig(config Config) string {
	return strings.Join([]string{
		config.String(),
		config.DatabaseURL().String(),
		config.StripeSecretKey().String(),
		config.StripeTimeout().String(),
		config.ResendAPIKey().String(),
		config.EmailFrom(),
	}, " ")
}
