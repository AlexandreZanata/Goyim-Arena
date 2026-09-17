package domain

import (
	"strings"
	"unicode/utf8"
)

// The supported interface locales of the MVP, in canonical BCP 47 form.
// The allowlist is a product decision mirrored by the CHECK constraint of
// app.profiles.interface_locale; adding a locale here also requires the
// catalog to support it (I18N_STANDARD.md).
const (
	LocaleBrazilianPortuguese = "pt-BR"
	LocaleAmericanEnglish     = "en-US"
)

var supportedLocaleTags = map[string]struct{}{
	LocaleBrazilianPortuguese: {},
	LocaleAmericanEnglish:     {},
}

// Locale is an immutable value object for the interface locale preference
// (interface_locale). It is deliberately independent from content_language:
// changing the UI locale never changes the language of Arena content.
//
// Only canonical tags are represented: "PT-br" parses to "pt-BR" and any
// tag outside the product allowlist is rejected instead of being reflected
// back to clients (I18N_STANDARD.md §4). Domain and application return
// stable values, never localized phrases.
type Locale struct {
	tag string
}

// ParseLocale canonicalizes and validates an interface locale preference.
// The raw input is trimmed, canonicalized as a language tag with an optional
// region and checked against the supported allowlist.
func ParseLocale(raw string) (Locale, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Locale{}, ErrEmptyLocale
	}

	canonical, err := canonicalizeLocaleTag(trimmed)
	if err != nil {
		return Locale{}, err
	}
	if _, ok := supportedLocaleTags[canonical]; !ok {
		return Locale{}, ErrUnsupportedLocale
	}
	return Locale{tag: canonical}, nil
}

// DefaultLocale returns the product default interface locale (pt-BR, never
// inferred from IP or geographic data).
func DefaultLocale() Locale {
	return Locale{tag: LocaleBrazilianPortuguese}
}

// SupportedLocales returns every interface locale the product accepts.
func SupportedLocales() []Locale {
	return []Locale{
		{tag: LocaleBrazilianPortuguese},
		{tag: LocaleAmericanEnglish},
	}
}

// canonicalizeLocaleTag accepts the tag shape the product needs — language
// plus optional region — and rejects everything else: empty or oversized
// values, non-ASCII input, private-use sequences, scripts, variants and
// extensions. The returned tag uses canonical case (language lowercase,
// region uppercase).
func canonicalizeLocaleTag(value string) (string, error) {
	for i := 0; i < len(value); i++ {
		if value[i] >= utf8.RuneSelf {
			return "", ErrInvalidLocale
		}
	}
	if value == "*" {
		return "", ErrInvalidLocale
	}

	subtags := strings.Split(value, "-")
	if len(subtags) > 2 {
		return "", ErrInvalidLocale
	}

	language := strings.ToLower(subtags[0])
	if len(language) < 2 || len(language) > 3 || !isASCIIAlpha(language) {
		return "", ErrInvalidLocale
	}
	canonical := language

	if len(subtags) == 2 {
		region := subtags[1]
		switch {
		case len(region) == 2 && isASCIIAlpha(region):
			canonical += "-" + strings.ToUpper(region)
		case len(region) == 3 && isASCIIDigit(region):
			canonical += "-" + region
		default:
			return "", ErrInvalidLocale
		}
	}

	return canonical, nil
}

func isASCIIAlpha(value string) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') {
			return false
		}
	}
	return len(value) > 0
}

func isASCIIDigit(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return len(value) > 0
}

// String returns the canonical BCP 47 tag.
func (l Locale) String() string {
	return l.tag
}

// IsSupported reports whether the locale is in the product allowlist.
func (l Locale) IsSupported() bool {
	_, ok := supportedLocaleTags[l.tag]
	return ok
}

// Equals reports whether two locales are the same canonical tag.
func (l Locale) Equals(other Locale) bool {
	return l.tag == other.tag
}

// IsZero reports whether the Locale is the uninitialized zero value.
func (l Locale) IsZero() bool {
	return l.tag == ""
}
