package i18n

import "strings"

// rtlPreferred lists the primary language subtags whose default script
// direction is right-to-left (CLDR default direction). It is a list of
// languages, not of locales: direction is a property of the script, and
// writing it per locale would duplicate it for every region.
//
// The list is deliberately conservative — it holds the languages this product
// could plausibly add, and everything else is left-to-right by rule. A locale
// that is missing from it is served `ltr`, which is what the four catalogs of
// today are; the pseudo-locale proves the attribute is rendered, and an RTL
// locale added later only has to appear here once.
var rtlPreferred = map[string]bool{
	"ar":  true, // Arabic
	"arc": true, // Aramaic
	"ckb": true, // Central Kurdish
	"dv":  true, // Divehi
	"fa":  true, // Persian
	"he":  true, // Hebrew
	"ks":  true, // Kashmiri
	"ps":  true, // Pashto
	"sd":  true, // Sindhi
	"ug":  true, // Uyghur
	"ur":  true, // Urdu
	"yi":  true, // Yiddish
}

// rtlScripts lists the ISO 15924 script codes written right-to-left. A tag
// that names one of them explicitly (`az-Arab`, `ku-Arab`) is right-to-left
// whatever the default direction of its language is, which is why the script
// is read before the language.
var rtlScripts = map[string]bool{
	"adlm": true, // Adlam
	"arab": true, // Arabic
	"hebr": true, // Hebrew
	"mand": true, // Mandaic
	"mend": true, // Mende Kikakui
	"nkoo": true, // N'Ko
	"samr": true, // Samaritan
	"syrc": true, // Syriac
	"thaa": true, // Thaana
}

// Direction is the writing direction of a locale: "rtl" or "ltr". It is the
// value of the document's `dir` attribute, and it ignores case and any region
// subtag, so `ar-EG`, `AR` and `ar` are one direction.
//
// The default is "ltr": an unknown or malformed tag must never make a page
// right-to-left, because a direction flip is a layout change, and a layout
// change driven by unvalidated input is a rendering decision taken by an
// attacker.
func Direction(locale string) string {
	parts := strings.Split(strings.ToLower(locale), "-")
	if len(parts) >= 2 && rtlScripts[parts[1]] {
		return "rtl"
	}
	if rtlPreferred[parts[0]] {
		return "rtl"
	}
	return "ltr"
}
