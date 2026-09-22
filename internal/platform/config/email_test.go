package config

import (
	"strconv"
	"strings"
	"testing"
)

// TestEmailConfigurationDefaultsToNothing covers the safe default: without the
// email variables the process still boots and no delivery path is invented —
// production then refuses the boot instead of serving forms whose links go
// nowhere.
func TestEmailConfigurationDefaultsToNothing(t *testing.T) {
	t.Parallel()

	config, err := Load(environ())
	if err != nil {
		t.Fatalf("load without email configuration: %v", err)
	}
	if config.ResendAPIKey().IsSet() {
		t.Fatal("no provider credential must be configured by default")
	}
	if config.EmailFrom() != "" {
		t.Fatalf("sender address = %q, want empty", config.EmailFrom())
	}
}

// TestEmailConfigurationAcceptsKeysAndSenderAddresses covers the accepted
// shapes: the provider sending key and the two sender forms RFC 5322 allows,
// with and without a display name.
func TestEmailConfigurationAcceptsKeysAndSenderAddresses(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"re_abc123", "re_live_with_underscores_and-dashes"} {
		config, err := Load(environ(ResendAPIKeyVariable + "=" + key))
		if err != nil {
			t.Fatalf("load with key %q: %v", key, err)
		}
		if string(config.ResendAPIKey().Unredacted()) != key {
			t.Fatal("the configured credential must be handed over exactly")
		}
	}

	for _, address := range []string{
		"no-reply@arena.invalid",
		"Arena <no-reply@arena.invalid>",
		"Arena Support <no-reply@arena.invalid>",
	} {
		config, err := Load(environ(EmailFromVariable + "=" + address))
		if err != nil {
			t.Fatalf("load with sender %q: %v", address, err)
		}
		if config.EmailFrom() != address {
			t.Fatalf("sender = %q, want the configured %q", config.EmailFrom(), address)
		}
	}
}

// TestEmailConfigurationRefusesUnusableValues covers every rejected shape: a
// value copied from another console (no provider prefix), a whitespace-padded
// paste, a credential that is not printable, and sender values that no message
// could carry. A refusal must name the variable, store nothing and never echo
// the value.
func TestEmailConfigurationRefusesUnusableValues(t *testing.T) {
	t.Parallel()

	credentialCases := []struct {
		name string
		raw  string
	}{
		{name: "missing provider prefix", raw: "sk_live_someone_elses_console"},
		{name: "missing prefix", raw: "abc123"},
		{name: "empty body", raw: "re_"},
		{name: "surrounding whitespace", raw: " re_abc123"},
		{name: "trailing newline", raw: "re_abc123\n"},
		{name: "too long", raw: "re_" + strings.Repeat("a", 513)},
		{name: "non ascii", raw: "re_ação"},
	}
	for _, testCase := range credentialCases {
		t.Run("credential/"+testCase.name, func(t *testing.T) {
			t.Parallel()

			config, err := Load(environ(ResendAPIKeyVariable + "=" + testCase.raw))
			if err == nil {
				t.Fatalf("value %q must be refused", testCase.raw)
			}
			if config.ResendAPIKey().IsSet() {
				t.Fatal("a refused credential must never be stored")
			}
			if !strings.Contains(err.Error(), ResendAPIKeyVariable) {
				t.Errorf("error must name the variable: %v", err)
			}
			if quoted := strconv.Quote(strings.TrimSpace(testCase.raw)); strings.Contains(err.Error(), quoted) {
				t.Errorf("error leaked the credential: %v", err)
			}
		})
	}

	senderCases := []struct {
		name string
		raw  string
	}{
		{name: "missing domain", raw: "no-reply"},
		{name: "missing local part", raw: "@arena.invalid"},
		{name: "trailing at sign", raw: "no-reply@"},
		{name: "angle brackets without address", raw: "Arena <>"},
		{name: "unclosed angle address", raw: "Arena <no-reply@arena.invalid"},
		{name: "unquoted display name", raw: "Arena no-reply@arena.invalid"},
		{name: "surrounding whitespace", raw: " no-reply@arena.invalid"},
		{name: "trailing newline", raw: "no-reply@arena.invalid\n"},
		{name: "control character in display name", raw: "Arena\tSupport <no-reply@arena.invalid>"},
		{name: "non ascii display name", raw: "Arená <no-reply@arena.invalid>"},
		{name: "too long", raw: strings.Repeat("a", 313) + "@arena.invalid"},
	}
	for _, testCase := range senderCases {
		t.Run("sender/"+testCase.name, func(t *testing.T) {
			t.Parallel()

			config, err := Load(environ(EmailFromVariable + "=" + testCase.raw))
			if err == nil {
				t.Fatalf("value %q must be refused", testCase.raw)
			}
			if config.EmailFrom() != "" {
				t.Fatal("a refused sender must never be stored")
			}
			if !strings.Contains(err.Error(), EmailFromVariable) {
				t.Errorf("error must name the variable: %v", err)
			}
		})
	}
}

// TestProductionRequiresTheEmailProvider pins the production safety rule: a
// registration is a link that has to arrive, so production refuses to boot
// without a delivery path. Each requirement is blanked on its own, so the
// refusal reported is the one under test.
func TestProductionRequiresTheEmailProvider(t *testing.T) {
	t.Parallel()

	for _, blank := range []string{ResendAPIKeyVariable + "=", EmailFromVariable + "="} {
		name, _, _ := strings.Cut(blank, "=")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config, err := Load(productionEnv(blank))
			if err == nil {
				t.Fatalf("production without %s must fail", name)
			}
			if !strings.Contains(err.Error(), name) ||
				!strings.Contains(err.Error(), "required when ARENA_ENV=production") {
				t.Fatalf("error must name the missing production requirement: %v", err)
			}
			if config.EmailFrom() != "" && name == EmailFromVariable {
				t.Fatal("a refused sender must never be stored")
			}
		})
	}

	// The same absent variables are the development baseline, where the local
	// email sink stands in for the provider.
	if _, err := Load(environ(EmailSinkDirVariable + "=/tmp/arena-email-sink")); err != nil {
		t.Fatalf("development must load without the provider configuration: %v", err)
	}
}

// TestEmailVariablesAreKnown keeps a typo from being silently ignored: a
// misspelled provider variable would otherwise leave production a requirement
// short without anyone noticing.
func TestEmailVariablesAreKnown(t *testing.T) {
	t.Parallel()

	for _, typo := range []string{"ARENA_RESEND_API_KEEY=re_abc123", "ARENA_EMAIL_FROMM=Arena <no-reply@arena.invalid>"} {
		name, _, _ := strings.Cut(typo, "=")
		_, err := Load(environ(typo))
		if err == nil {
			t.Fatalf("the typo %s must fail the load", name)
		}
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error must name the unknown variable: %v", err)
		}
	}
}

// TestResendCredentialNeverPrints proves the provider credential follows the
// Secret redaction rules while the sender address stays readable: the address
// is the value every recipient sees, so an operator must be able to confirm it.
func TestResendCredentialNeverPrints(t *testing.T) {
	t.Parallel()

	const credential = "re_live_never_print_me"
	const sender = "Arena Support <no-reply@arena.invalid>"
	config, err := Load(productionEnv(ResendAPIKeyVariable + "=" + credential))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for index, rendered := range []string{
		config.String(),
		config.ResendAPIKey().String(),
		renderedConfig(config),
	} {
		if strings.Contains(rendered, credential) {
			t.Errorf("rendering %d leaked the credential: %q", index, rendered)
		}
	}
	if string(config.ResendAPIKey().Unredacted()) != credential {
		t.Fatal("Unredacted must expose the exact credential to the composition root")
	}

	// The sender address is configuration, not a secret: a rendering that
	// reaches every field must show it, because it is the value every
	// recipient sees and an operator has to be able to confirm it.
	readable, err := Load(productionEnv(EmailFromVariable + "=" + sender))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.Contains(renderedConfig(readable), sender) {
		t.Errorf("the sender address must be readable in the rendered config: %q", renderedConfig(readable))
	}
}
