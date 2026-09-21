//go:build pseudolocale

// The tagged build is the one the browser gate runs: it must carry the
// pseudo-locale, negotiate-able like any other, and it must leave the real
// catalogs untouched (P18-T10).
package i18n

import (
	"strings"
	"testing"
)

func TestTaggedBuildCarriesThePseudoLocale(t *testing.T) {
	t.Parallel()

	if !PseudoEnabled {
		t.Fatal("PseudoEnabled is false in a build with the pseudolocale tag")
	}

	found := false
	for _, locale := range SupportedLocales() {
		if locale == PseudoLocale {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s is missing from the allowlist of the tagged build: %v", PseudoLocale, SupportedLocales())
	}

	message, err := Message(PseudoLocale, "auth.brand")
	if err != nil {
		t.Fatalf("Message(%s): %v", PseudoLocale, err)
	}
	if !strings.HasPrefix(message, pseudoOpen) || !strings.HasSuffix(message, pseudoClose) {
		t.Errorf("Message(%s, auth.brand) = %q, want the pseudo markers", PseudoLocale, message)
	}
}

// TestTaggedBuildFormatsPseudoPlaceholders is the contract the derivation
// exists to keep: the pseudo catalog is a catalog like any other, so
// everything that formats a message works on it unchanged.
func TestTaggedBuildFormatsPseudoPlaceholders(t *testing.T) {
	t.Parallel()

	formatted, err := Format(PseudoLocale, "arenas.document.page_title", map[string]string{"subject": "A AGI existirá até 2040"})
	if err != nil {
		t.Fatalf("Format(%s): %v", PseudoLocale, err)
	}
	if strings.Contains(formatted, "{subject}") {
		t.Errorf("Format(%s) = %q, placeholder was not replaced", PseudoLocale, formatted)
	}
	if !strings.Contains(formatted, "A AGI existirá até 2040") {
		t.Errorf("Format(%s) = %q, missing the substituted subject", PseudoLocale, formatted)
	}
	if _, err := Format(PseudoLocale, "arenas.document.page_title", nil); err == nil {
		t.Error("a missing placeholder value must fail in the pseudo locale too")
	}
}

// TestTaggedBuildLeavesTheRealCatalogsAlone keeps the derivation from leaking
// into the locales a person reads: the pseudo catalog is an addition, never a
// rewrite.
func TestTaggedBuildLeavesTheRealCatalogsAlone(t *testing.T) {
	t.Parallel()

	for _, locale := range []string{DefaultLocale, "en-US"} {
		message, err := Message(locale, "auth.brand")
		if err != nil {
			t.Fatalf("Message(%s): %v", locale, err)
		}
		if strings.Contains(message, pseudoOpen) || strings.Contains(message, "•") {
			t.Errorf("Message(%s, auth.brand) = %q carries pseudo text", locale, message)
		}
	}
}
