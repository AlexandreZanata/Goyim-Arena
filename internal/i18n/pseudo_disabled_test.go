//go:build !pseudolocale

// The delivered build must not carry the pseudo-locale (P18-T10). This file is
// the proof, and it compiles only in the build that promises it: a Makefile,
// a CI job or a comment cannot make that promise, a build tag can.
package i18n

import (
	"testing"
)

func TestDefaultBuildDoesNotCarryThePseudoLocale(t *testing.T) {
	t.Parallel()

	if PseudoEnabled {
		t.Fatal("PseudoEnabled is true in a build without the pseudolocale tag")
	}
	for _, locale := range SupportedLocales() {
		if locale == PseudoLocale {
			t.Fatalf("%s is served by a build that must not carry it: %v", PseudoLocale, SupportedLocales())
		}
	}
	if _, err := Message(PseudoLocale, "auth.brand"); err == nil {
		t.Errorf("Message(%s) succeeded in a build without its catalog", PseudoLocale)
	}
}
