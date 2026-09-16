// Package i18ngen is the internal generator of typed locale catalogs
// (P02-T07), per I18N_STANDARD.md.
//
// It parses locales/<BCP-47>/<namespace>.json files, validates unknown
// locales, invalid JSON, duplicate keys, key parity across locales and
// placeholder parity, then emits deterministic artifacts: TypeScript keys
// for the frontend and an embedded Go catalog for HTML and emails.
//
// Generated code never receives manual edits; drift is detected by
// make generate-check. This is build tooling: nothing here runs in the
// request path.
package i18ngen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SupportedLocales is the initial interface locale allowlist (BCP 47
// canonical tags), per I18N_STANDARD.md.
var SupportedLocales = []string{"en-US", "pt-BR"}

// DefaultLocale is the product default.
const DefaultLocale = "pt-BR"

// Catalog is one parsed namespace of one locale: a flat map of dot-joined
// key paths to message values. Nesting in JSON is layout, not identity.
type Catalog map[string]string

// Message is one validated catalog entry.
type Message struct {
	Locale       string
	Namespace    string
	Key          string
	Value        string
	Placeholders []string
}

// Bundle is the fully parsed and ordered state of the locales tree.
type Bundle struct {
	// Namespaces are the namespace names present in every locale.
	Namespaces []string

	// Messages maps namespace -> key -> locale -> value.
	Messages map[string]map[string]map[string]string

	// Placeholders maps namespace -> key -> sorted placeholder names.
	Placeholders map[string]map[string][]string
}

// ParseCatalogFile reads one locales/<locale>/<namespace>.json file into a
// flat dot-joined catalog, rejecting invalid JSON and duplicate keys.
func ParseCatalogFile(path string) (Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("i18ngen: read %s: %w", path, err)
	}

	flat := map[string]string{}
	if err := flattenJSON(data, nil, flat, path); err != nil {
		return nil, err
	}
	return flat, nil
}

// flattenJSON walks raw JSON, recording leaf strings as dot-joined keys.
// Duplicate keys are rejected: encoding/json keeps the last silently, which
// would hide editorial mistakes.
func flattenJSON(data []byte, prefix []string, into map[string]string, path string) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return fmt.Errorf("i18ngen: %s is empty", path)
	}

	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return fmt.Errorf("i18ngen: %s is not valid JSON: %w", path, err)
	}
	if decoder.More() {
		return fmt.Errorf("i18ngen: %s has trailing content after the JSON document", path)
	}

	return flattenValue(tree, prefix, into, path)
}

func flattenValue(value any, prefix []string, into map[string]string, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			joined := append(append([]string{}, prefix...), key)
			if err := flattenValue(typed[key], joined, into, path); err != nil {
				return err
			}
		}
	case string:
		key := strings.Join(prefix, ".")
		if _, duplicate := into[key]; duplicate {
			return fmt.Errorf("i18ngen: %s defines duplicate key %q", path, key)
		}
		into[key] = typed
	case nil:
		return fmt.Errorf("i18ngen: %s has null value at %q", path, strings.Join(prefix, "."))
	default:
		return fmt.Errorf("i18ngen: %s has non-string value at %q", path, strings.Join(prefix, "."))
	}
	return nil
}

// SplitKey splits "namespace.rest.of.key" into its namespace and the rest.
func SplitKey(key string) (namespace, remainder string) {
	first := strings.IndexByte(key, '.')
	if first < 0 {
		return key, ""
	}
	return key[:first], key[first+1:]
}

// LocaleDir returns the directory of one locale inside the locales root.
func LocaleDir(root, locale string) string {
	return filepath.Join(root, locale)
}
