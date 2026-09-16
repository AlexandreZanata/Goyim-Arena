// Package i18n exposes the typed locale catalog to inbound adapters
// (HTML, emails), per I18N_STANDARD.md section 5: domain and application
// return codes, presentation localizes through this package.
//
// The message data lives in generated.go (do not edit) and is regenerated
// from locales/ by make generate. Lookup is locale-exact: the fallback to
// the default locale is decided by the caller (the presentation layer),
// which also owns locale resolution.
package i18n

import (
	"fmt"
	"sort"
)

// The catalog and placeholders variables are declared in generated.go,
// produced from locales/ by internal/i18ngen (make generate) and never
// edited manually.
//
//go:generate go run ../cmd/i18ngen

// SupportedLocales lists the interface locales with catalogs present.
func SupportedLocales() []string {
	locales := make([]string, 0, len(catalog))
	for locale := range catalog {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return locales
}

// Message returns the localized message for key in locale. It fails when
// the locale or key is unknown: callers resolve the locale against the
// allowlist first, so a miss here is a programming error, not a runtime
// event. The default locale is never silently substituted — production
// fallback belongs to the presentation layer, with a metric, per the
// I18N standard.
func Message(locale, key string) (string, error) {
	messages, ok := catalog[locale]
	if !ok {
		return "", fmt.Errorf("i18n: unknown locale %q", locale)
	}
	message, ok := messages[key]
	if !ok {
		return "", fmt.Errorf("i18n: unknown message key %q for locale %q", key, locale)
	}
	return message, nil
}

// Placeholders returns the sorted placeholder names of a key.
func Placeholders(key string) []string {
	return placeholders[key]
}

// DefaultLocale is the product default per I18N_STANDARD.md.
const DefaultLocale = "pt-BR"
