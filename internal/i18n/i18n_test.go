// Tests of the generated catalog consumer (P02-T07): the artifact emitted
// by internal/i18ngen must be consumable in every supported locale, with
// strict failures for unknown locales or keys.
package i18n

import (
	"strings"
	"testing"
)

func TestMessageReturnsBothLocales(t *testing.T) {
	t.Parallel()

	for _, locale := range SupportedLocales() {
		title, err := Message(locale, "errors.internal.title")
		if err != nil {
			t.Fatalf("Message(%s): %v", locale, err)
		}
		if strings.Contains(title, "errors.internal.title") {
			t.Errorf("raw key leaked into message: %q", title)
		}
	}

	if got, _ := Message("en-US", "errors.internal.title"); got != "Internal error" {
		t.Errorf("en-US title = %q, want the generated source value", got)
	}
	if got, _ := Message("pt-BR", "errors.internal.title"); got != "Erro interno" {
		t.Errorf("pt-BR title = %q, want the generated source value", got)
	}
}

func TestMessageRejectsUnknownLocaleAndKey(t *testing.T) {
	t.Parallel()

	if _, err := Message("xx-XX", "errors.internal.title"); err == nil {
		t.Error("unknown locale should fail")
	}
	if _, err := Message("pt-BR", "errors.ghost.title"); err == nil {
		t.Error("unknown key should fail")
	}
}

func TestPlaceholdersAndDefaultLocale(t *testing.T) {
	t.Parallel()

	if names := Placeholders("errors.internal.title"); len(names) != 0 {
		t.Errorf("expected no placeholders on errors.internal.title, got %v", names)
	}
	if DefaultLocale != "pt-BR" {
		t.Errorf("DefaultLocale = %q, want pt-BR", DefaultLocale)
	}
}
