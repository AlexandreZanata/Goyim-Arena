// Tests of the document direction (P18-T10): the `dir` attribute of a page is
// derived from the locale, and an unknown tag must never flip a layout.
package i18n

import "testing"

func TestDirectionFollowsTheScriptOfTheLanguage(t *testing.T) {
	t.Parallel()

	rightToLeft := []string{"ar", "ar-EG", "HE", "fa-IR", "ur-PK", "ckb", "ps", "sd", "ug", "yi", "dv", "ks", "arc", "az-Arab", "ku-arab"}
	for _, locale := range rightToLeft {
		if got := Direction(locale); got != "rtl" {
			t.Errorf("Direction(%q) = %q, want rtl", locale, got)
		}
	}

	leftToRight := []string{DefaultLocale, "en-US", "es-419", PseudoLocale, "ku", "az-Latn", "", "x", "not-a-locale-but-ltr"}
	for _, locale := range leftToRight {
		if got := Direction(locale); got != "ltr" {
			t.Errorf("Direction(%q) = %q, want ltr", locale, got)
		}
	}
}

// TestDirectionNeverDependsOnRegionOrCase pins the shape of the comparison:
// direction is a property of the language, so the region and the case of the
// tag cannot change the answer.
func TestDirectionNeverDependsOnRegionOrCase(t *testing.T) {
	t.Parallel()

	if Direction("ar") != Direction("ar-SA") || Direction("ar") != Direction("AR-sa") {
		t.Errorf("direction of the Arabic locales disagrees: %q, %q, %q",
			Direction("ar"), Direction("ar-SA"), Direction("AR-sa"))
	}
}
