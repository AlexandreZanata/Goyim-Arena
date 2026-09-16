// Strict validation of the locales tree (P02-T07): unknown locales are
// rejected, duplicate JSON keys are detected on the raw token stream
// (encoding/json alone collapses them silently), and key/placeholder
// parity is enforced across locales, per I18N_STANDARD.md section 8
// (development and CI fail on missing, extra or divergent keys).
package i18ngen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// placeholderPattern matches named placeholders like {userName}.
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// strayBracePattern reports braces that are not part of a valid named
// placeholder, so malformed templates fail the build instead of rendering.
var strayBracePattern = regexp.MustCompile(`\{[^{}]*\}|[{}]`)

// LoadBundle parses and validates the whole locales tree at root.
func LoadBundle(root string) (*Bundle, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("i18ngen: read locales root %s: %w", root, err)
	}

	found := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !isSupportedLocale(entry.Name()) {
			return nil, fmt.Errorf("i18ngen: unknown locale directory %q (allowed: %s)",
				entry.Name(), strings.Join(SupportedLocales, ", "))
		}
		found[entry.Name()] = true
	}
	for _, locale := range SupportedLocales {
		if !found[locale] {
			return nil, fmt.Errorf("i18ngen: required locale %q is missing under %s", locale, root)
		}
	}

	bundle := &Bundle{
		Messages:     map[string]map[string]map[string]string{},
		Placeholders: map[string]map[string][]string{},
	}
	namespaceFiles := map[string][]string{} // namespace -> locales declaring it

	for _, locale := range SupportedLocales {
		files, err := os.ReadDir(filepath.Join(root, locale))
		if err != nil {
			return nil, fmt.Errorf("i18ngen: read locale %s: %w", locale, err)
		}
		for _, file := range files {
			name := file.Name()
			if file.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			namespace := strings.TrimSuffix(name, ".json")
			namespaceFiles[namespace] = append(namespaceFiles[namespace], locale)

			path := filepath.Join(root, locale, name)
			if err := scanDuplicateKeysFile(path); err != nil {
				return nil, err
			}
			catalog, err := ParseCatalogFile(path)
			if err != nil {
				return nil, err
			}
			if err := bundle.absorb(locale, namespace, catalog, path); err != nil {
				return nil, err
			}
		}
	}

	// Namespace parity: every locale declares the same namespace files.
	bundle.Namespaces = make([]string, 0, len(namespaceFiles))
	for namespace, locales := range namespaceFiles {
		if len(locales) != len(SupportedLocales) {
			return nil, fmt.Errorf("i18ngen: namespace %q is not declared by every locale (found: %s)",
				namespace, strings.Join(locales, ", "))
		}
		bundle.Namespaces = append(bundle.Namespaces, namespace)
	}
	sort.Strings(bundle.Namespaces)

	if err := bundle.checkKeyParity(); err != nil {
		return nil, err
	}
	return bundle, nil
}

// absorb merges one parsed catalog into the bundle, validating placeholder
// shapes and cross-locale placeholder parity.
func (bundle *Bundle) absorb(locale, namespace string, catalog Catalog, path string) error {
	if bundle.Messages[namespace] == nil {
		bundle.Messages[namespace] = map[string]map[string]string{}
	}
	for _, key := range sortedKeys(catalog) {
		value := catalog[key]
		if bundle.Messages[namespace][key] == nil {
			bundle.Messages[namespace][key] = map[string]string{}
		}

		placeholders, err := extractPlaceholders(value)
		if err != nil {
			return fmt.Errorf("i18ngen: %s key %q: %w", path, key, err)
		}
		if bundle.Placeholders[namespace] == nil {
			bundle.Placeholders[namespace] = map[string][]string{}
		}
		if existing, seen := bundle.Placeholders[namespace][key]; seen {
			if strings.Join(existing, ",") != strings.Join(placeholders, ",") {
				return fmt.Errorf("i18ngen: key %q has placeholder divergence between locales: [%s] vs [%s]",
					key, strings.Join(existing, ", "), strings.Join(placeholders, ", "))
			}
		} else {
			bundle.Placeholders[namespace][key] = placeholders
		}
		bundle.Messages[namespace][key][locale] = value
	}
	return nil
}

// checkKeyParity fails when locales disagree on the key set of a namespace.
func (bundle *Bundle) checkKeyParity() error {
	for _, namespace := range bundle.Namespaces {
		var reference []string
		for key := range bundle.Messages[namespace] {
			reference = append(reference, key)
		}
		sort.Strings(reference)
		for _, key := range reference {
			for _, locale := range SupportedLocales {
				if _, ok := bundle.Messages[namespace][key][locale]; !ok {
					return fmt.Errorf("i18ngen: key %q is missing in locale %s (namespace %s)",
						key, locale, namespace)
				}
			}
		}
	}
	return nil
}

// extractPlaceholders returns the sorted placeholder names of a message
// and rejects malformed braces so templates fail loudly.
func extractPlaceholders(value string) ([]string, error) {
	names := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(value, -1) {
		names[match[1]] = true
	}
	clean := placeholderPattern.ReplaceAllString(value, "")
	if stray := strayBracePattern.FindAllString(clean, -1); len(stray) > 0 {
		return nil, fmt.Errorf("malformed placeholder(s) %s", strings.Join(stray, " "))
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// scanDuplicateKeysFile rejects duplicate keys by walking the raw JSON
// token stream: encoding/json alone would keep the last occurrence.
func scanDuplicateKeysFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("i18ngen: read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, path, nil); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("i18ngen: %s has trailing content after the JSON document", path)
	}
	return nil
}

// scanJSONValue consumes one JSON value from the decoder, tracking object
// key uniqueness down the path.
func scanJSONValue(decoder *json.Decoder, path string, keyPath []string) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("i18ngen: %s: invalid JSON: %w", path, err)
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			return scanJSONObject(decoder, path, keyPath)
		case '[':
			index := 0
			for decoder.More() {
				if err := scanJSONValue(decoder, path, append(keyPath, fmt.Sprint(index))); err != nil {
					return err
				}
				index++
			}
			closing, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("i18ngen: %s: invalid JSON: %w", path, err)
			}
			if closing != json.Delim(']') {
				return fmt.Errorf("i18ngen: %s: invalid JSON: expected ]", path)
			}
			return nil
		default:
			return fmt.Errorf("i18ngen: %s: invalid JSON: unexpected delimiter %v", path, delimiter)
		}
	}
	return nil // Scalar leaf: nothing to track.
}

// scanJSONObject consumes one JSON object, rejecting duplicate keys.
func scanJSONObject(decoder *json.Decoder, path string, keyPath []string) error {
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("i18ngen: %s: invalid JSON: %w", path, err)
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("i18ngen: %s: invalid JSON: object key is not a string", path)
		}
		if seen[key] {
			joined := strings.Join(append(append([]string{}, keyPath...), key), ".")
			return fmt.Errorf("i18ngen: %s defines duplicate key %q", path, joined)
		}
		seen[key] = true
		if err := scanJSONValue(decoder, path, append(keyPath, key)); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("i18ngen: %s: invalid JSON: %w", path, err)
	}
	if closing != json.Delim('}') {
		return fmt.Errorf("i18ngen: %s: invalid JSON: expected }", path)
	}
	return nil
}

// sortedKeys returns the sorted keys of a catalog.
func sortedKeys(catalog Catalog) []string {
	keys := make([]string, 0, len(catalog))
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// isSupportedLocale reports whether the tag is in the initial allowlist.
func isSupportedLocale(tag string) bool {
	for _, locale := range SupportedLocales {
		if locale == tag {
			return true
		}
	}
	return false
}
