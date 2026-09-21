// Pseudo-locale of the layout and hardcoded-text gate (P18-T10).
//
// The pseudo-locale is a derived catalog, never a second source of truth: it
// is built from the messages of a real locale by this file, so a translation
// that lands in locales/ is covered by the gate in the same commit that adds
// it, and a translator is never asked to maintain two copies of one sentence.
//
// It exists to break three assumptions that a pt-BR-only development habit
// hides: that a message is short enough to fit its control, that prose is
// ASCII, and that the interface language of a document is Portuguese. Every
// message therefore leaves this file bracketed by ⟦ ⟧ (so the gate can prove
// the pseudo catalog reached the page, not only the `lang` attribute), with
// its letters accented (so a font that cannot render a diacritic shows up as a
// broken page instead of a silent one) and padded so the text is measurably
// longer than its source.
//
// The derivation is deliberately a pure function of one catalog: it is what
// the unit suite proves, and it is what makes the pseudo catalog reproducible
// byte for byte on every machine.
//
// The catalog itself is registered only in a build that asks for it
// (pseudo_enabled.go, behind the `pseudolocale` build tag): the delivered
// binary cannot serve a locale it does not carry, so "only for CI and
// development" is a property of the build, not a promise in a comment.
package i18n

import (
	"strings"
	"unicode/utf8"
)

const (
	// PseudoLocale is the pseudo-locale tag. `qps` is the ISO 639 code
	// reserved for pseudo-languages, and the tag is deliberately outside
	// the language/region shape a translator could mistake for a real
	// interface locale.
	PseudoLocale = "qps-Ploc"

	// pseudoOpen and pseudoClose bracket every derived message, so the
	// browser gate can assert that the pseudo catalog was served.
	pseudoOpen  = "⟦"
	pseudoClose = "⟧"

	// pseudoPadding is the filler appended to every derived message. It is
	// a repeated non-ASCII glyph rather than a repeated letter: the width
	// it adds is what the layout gate measures, and a filler that reads
	// like a word would invite someone to translate it.
	pseudoPadding = "••••"

	// pseudoGrowth is the minimum growth the derived message has over its
	// source, as a fraction of the source length. Every catalog message
	// grows by at least this much; the unit suite asserts it, so a future
	// change to the transform cannot quietly stop exercising the layout.
	pseudoGrowth = 0.4
)

// pseudoAccents is the byte-for-byte deterministic accent table of the
// derivation. Only Latin-1 glyphs are used: a pseudo message rendered in a
// font without them would be a false positive of the gate.
var pseudoAccents = map[rune]rune{
	'a': 'á', 'A': 'Á',
	'e': 'é', 'E': 'É',
	'i': 'í', 'I': 'Í',
	'o': 'ó', 'O': 'Ó',
	'u': 'ú', 'U': 'Ú',
	'c': 'ç', 'C': 'Ç',
	'n': 'ñ', 'N': 'Ñ',
	'y': 'ý', 'Y': 'Ý',
}

// DerivePseudo returns the pseudo catalog of a source catalog: the same keys,
// every message transformed by PseudoMessage. The source is never modified,
// and a caller that mutates the result cannot affect it.
func DerivePseudo(messages map[string]string) map[string]string {
	derived := make(map[string]string, len(messages))
	for key, message := range messages {
		derived[key] = PseudoMessage(message)
	}
	return derived
}

// PseudoMessage transforms one message: bracketed, accented and padded, with
// every `{placeholder}` of the source preserved character for character.
//
// Placeholders are copied verbatim because the derivation must not break the
// contract the catalog declares: `Format` looks for the exact name, and a
// pseudo catalog that renames `{max}` to `{máx}` would make every formatted
// message fail in the only build that renders it.
func PseudoMessage(message string) string {
	var out strings.Builder
	out.Grow(len(message) + len(pseudoOpen) + len(pseudoClose) + len(pseudoPadding))

	out.WriteString(pseudoOpen)
	insidePlaceholder := false
	for _, character := range message {
		switch {
		case character == '{':
			insidePlaceholder = true
			out.WriteRune(character)
		case character == '}' && insidePlaceholder:
			insidePlaceholder = false
			out.WriteRune(character)
		case insidePlaceholder:
			out.WriteRune(character)
		default:
			if accented, ok := pseudoAccents[character]; ok {
				out.WriteRune(accented)
			} else {
				out.WriteRune(character)
			}
		}
	}

	// The padding is proportional to the whole message, placeholders
	// included: a label that is mostly one placeholder (`{relation}:
	// {excerpt}`) must grow like a label that is mostly prose, otherwise the
	// growth of the pseudo text would depend on how the sentence happens to
	// be written and the assertion below would be a coin flip.
	for written := 0; written < (utf8.RuneCountInString(message)+1)/2; written += len(pseudoPadding) {
		out.WriteString(pseudoPadding)
	}

	out.WriteString(pseudoClose)
	return out.String()
}

// IsPseudoLocale reports whether a locale is the pseudo-locale of the gate. It
// is a comparison, not a lookup: the tag is meaningful in every build, and a
// build without the catalog must still be able to refuse it explicitly.
func IsPseudoLocale(locale string) bool {
	return locale == PseudoLocale
}
