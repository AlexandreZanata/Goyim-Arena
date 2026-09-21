//go:build pseudolocale

package i18n

// PseudoEnabled reports whether this build carries the pseudo-locale catalog.
// It is true here and only here: the tag is the whole switch.
const PseudoEnabled = true

// init registers the derived pseudo catalog before any caller can read the
// catalog: package initialization runs before main and before any test, so the
// locale allowlist that the presentation layer derives from
// SupportedLocales() sees the pseudo-locale from its first call, and a
// registration that happened later would be a locale nobody can negotiate.
//
// The source is always the default locale, which is also the locale the
// product promises to be complete: deriving from a partially translated
// catalog would make missing messages look like a derivation bug.
func init() {
	source, ok := catalog[DefaultLocale]
	if !ok {
		panic("i18n: the default locale has no catalog to derive the pseudo-locale from")
	}
	catalog[PseudoLocale] = DerivePseudo(source)
}
