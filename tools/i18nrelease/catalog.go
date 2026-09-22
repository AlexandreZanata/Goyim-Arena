// Loading and measuring of the locale catalogs (P20-T09).
//
// The generator of P02-T07 already refuses a locales tree that drifts, and this
// package deliberately does not call it: the audit measures the tree a second
// time, independently, because a gate that asks the producer whether the
// product is right is a gate that agrees with itself. Reading the JSON here and
// comparing the answer with the generator's is what makes the register's
// numbers evidence instead of a summary of the tool that wrote them.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// defaultLocale is the editorial source of the catalog (I18N_STANDARD.md §10):
// every other locale is measured against it.
const defaultLocale = "pt-BR"

// placeholderPattern matches named placeholders, exactly as the generator does.
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// catalog is one locales tree: every locale, every namespace, every message.
type catalog struct {
	// locales is the sorted list of locale directories that hold messages.
	locales []string
	// messages[locale][key] is one flattened message; keys are dot-joined
	// across nested JSON objects, the shape the generator produces.
	messages map[string]map[string]string
	// namespaces[locale] is the sorted list of namespaces of a locale.
	namespaces map[string][]string
	// placeholders[key] is the sorted placeholder set of one key.
	placeholders map[string][]string
}

// loadCatalog reads every locale of root/locales.
//
// It fails on anything it cannot read or parse: a locale tree the audit cannot
// see is not a clean tree, it is an unmeasured one, and the difference is the
// whole point of the task.
func loadCatalog(root string) (*catalog, error) {
	localesRoot := filepath.Join(root, "locales")
	entries, err := os.ReadDir(localesRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", localesRoot, err)
	}

	catalog := &catalog{
		messages:     map[string]map[string]string{},
		namespaces:   map[string][]string{},
		placeholders: map[string][]string{},
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		locale := entry.Name()
		catalog.locales = append(catalog.locales, locale)

		files, err := os.ReadDir(filepath.Join(localesRoot, locale))
		if err != nil {
			return nil, fmt.Errorf("read locale %s: %w", locale, err)
		}
		messages := map[string]string{}
		for _, file := range files {
			name := file.Name()
			if file.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
			namespace := strings.TrimSuffix(name, ".json")
			catalog.namespaces[locale] = append(catalog.namespaces[locale], namespace)

			raw, err := os.ReadFile(filepath.Join(localesRoot, locale, name))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", filepath.Join(locale, name), err)
			}
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				return nil, fmt.Errorf("parse %s: %w", filepath.Join(locale, name), err)
			}
			// The file's root object is the namespace itself (`arenas.json` holds
			// `{"arenas": {...}}`), and the generator's keys start with that same
			// name: flattening from the root, not from an injected prefix, is what
			// makes the keys this audit compares with the source the keys the
			// product actually asks for.
			if len(document) != 1 {
				return nil, fmt.Errorf("%s: the root must hold exactly the namespace %q", filepath.Join(locale, name), namespace)
			}
			for root := range document {
				if root != namespace {
					return nil, fmt.Errorf("%s: the root object is %q and the file is named %q", filepath.Join(locale, name), root, namespace)
				}
			}
			if err := flatten(messages, namespace, "", document); err != nil {
				return nil, err
			}
		}
		sort.Strings(catalog.namespaces[locale])
		if len(messages) == 0 {
			return nil, fmt.Errorf("locale %s declares no message", locale)
		}
		catalog.messages[locale] = messages
	}
	sort.Strings(catalog.locales)

	// The placeholder set of a key is the one the **default locale** declares,
	// which is what makes every other locale comparable against a fixed
	// reference instead of against whichever directory the filesystem listed
	// first. A key the default locale does not declare falls back to the first
	// locale that has it, and the register counts it as an extra key.
	for _, key := range catalog.allKeys() {
		if message, ok := catalog.messages[defaultLocale][key]; ok {
			catalog.placeholders[key] = placeholderNames(message)
			continue
		}
		for _, locale := range catalog.locales {
			if message, ok := catalog.messages[locale][key]; ok {
				catalog.placeholders[key] = placeholderNames(message)
				break
			}
		}
	}
	if len(catalog.locales) == 0 {
		return nil, fmt.Errorf("%s holds no locale", localesRoot)
	}
	return catalog, nil
}

// flatten walks one namespace document, joining nested objects with dots. A
// leaf that is not a string is refused: the catalog holds messages, and a
// number or a nested array in its place would be a message no locale can
// render.
func flatten(target map[string]string, key, path string, node any) error {
	switch value := node.(type) {
	case map[string]any:
		if len(value) == 0 {
			return fmt.Errorf("%s: namespace object %q is empty", key, path)
		}
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			child := name
			if path != "" {
				child = path + "." + name
			}
			if err := flatten(target, key, child, value[name]); err != nil {
				return err
			}
		}
	case string:
		target[path] = value
	default:
		return fmt.Errorf("%s: %q is a %T, not a message", key, path, node)
	}
	return nil
}

// placeholderNames returns the sorted placeholder names of one message.
func placeholderNames(message string) []string {
	found := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(message, -1) {
		found[match[1]] = true
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// keys returns the sorted keys of one locale.
func (catalog *catalog) keys(locale string) []string {
	keys := make([]string, 0, len(catalog.messages[locale]))
	for key := range catalog.messages[locale] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// allKeys returns every key any locale declares, sorted.
func (catalog *catalog) allKeys() []string {
	seen := map[string]bool{}
	for _, messages := range catalog.messages {
		for key := range messages {
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// namespaceNames returns every namespace any locale declares, sorted.
func (catalog *catalog) namespaceNames() []string {
	seen := map[string]bool{}
	for _, namespaces := range catalog.namespaces {
		for _, namespace := range namespaces {
			seen[namespace] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// declareNamespace is a namespace literal the delivered source carries, and the
// locale tag of one message file: the two facts the register compares with the
// tree.
type sourceReferences struct {
	// literals is every dotted catalog key literal found in delivered source,
	// with the file that carries it.
	literals map[string]string
	// prefixes is every trailing-dot prefix literal, which is how a call site
	// composes a key from a stable stem (`"arenas.document.status."+status`).
	prefixes map[string]string
	// contentLanguageSeed is the language `tools/e2e/seed` declares when it
	// creates the arena the journeys read.
	contentLanguageSeed string
}

// scanDeliveredSource walks the delivered tree once, collecting the catalog
// literals and the seeded content language.
//
// Generated artifacts are skipped on purpose: they carry every key by
// construction, so counting them would make every key referenced and the
// measurement would be a restatement of the generator.
func scanDeliveredSource(root string, catalog *catalog) (sourceReferences, error) {
	references := sourceReferences{
		literals: map[string]string{},
		prefixes: map[string]string{},
	}
	namespaces := catalog.namespaceNames()
	if len(namespaces) == 0 {
		return references, fmt.Errorf("the catalog declares no namespace")
	}
	// The namespace alternation is grouped on purpose: without it the trailing
	// segments would attach to the last alternative alone, and every key of
	// every other namespace would be read as a bare namespace name — which is
	// how a scan reports 193 keys as unreachable while the tree spells them out.
	keyPattern := regexp.MustCompile(`"((?:` + strings.Join(namespaces, "|") + `)(?:\.[A-Za-z0-9_]+)*\.?)"`)

	for _, directory := range []string{"internal", "cmd", "tools", "web/src", "tests"} {
		base := filepath.Join(root, directory)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "node_modules" || name == "dist" || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			switch {
			case relative == filepath.Join("internal", "i18n", "generated.go"):
				return nil
			case relative == filepath.Join("web", "src", "i18n", "generated.ts"):
				return nil
			case strings.HasPrefix(relative, filepath.Join("tools", "i18nrelease")):
				// The audit's own directory is skipped: its falsification fixtures
				// spell keys that no locale declares on purpose, and a measurement
				// that counted them would report its own tests as defects of the
				// product. The absence is declared in the register.
				return nil
			case !hasSourceSuffix(entry.Name()):
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(raw)
			if strings.HasSuffix(entry.Name(), "main.go") && strings.Contains(relative, filepath.Join("tools", "e2e", "seed")) {
				if language := seedLanguage(text); language != "" {
					references.contentLanguageSeed = language
				}
			}
			for _, match := range keyPattern.FindAllStringSubmatch(text, -1) {
				literal := match[1]
				if strings.HasSuffix(literal, ".") {
					if _, seen := references.prefixes[literal]; !seen {
						references.prefixes[literal] = relative
					}
					continue
				}
				if _, seen := references.literals[literal]; !seen {
					references.literals[literal] = relative
				}
			}
			return nil
		})
		if err != nil {
			return references, err
		}
	}
	return references, nil
}

// hasSourceSuffix reports whether a file is part of the source the audit reads.
func hasSourceSuffix(name string) bool {
	for _, suffix := range []string{".go", ".ts", ".js", ".html"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// seedLanguage returns the language literal `tools/e2e/seed` declares, so the
// audit can compare it with the value the harness exports.
func seedLanguage(source string) string {
	match := regexp.MustCompile(`ParseLanguage\("([a-zA-Z-]+)"\)`).FindStringSubmatch(source)
	if match == nil {
		return ""
	}
	return match[1]
}

// harnessContentLanguage returns the content language the harness exports.
func harnessContentLanguage(source string) string {
	match := regexp.MustCompile(`(?m)^CONTENT_LANGUAGE="([a-zA-Z-]+)"$`).FindStringSubmatch(source)
	if match == nil {
		return ""
	}
	return match[1]
}

// journeyLocales returns the interface locales the browser journeys iterate
// over, read from the module that declares them.
func journeyLocales(source string) []string {
	match := regexp.MustCompile(`JOURNEY_LOCALES = \[([^\]]*)\]`).FindStringSubmatch(source)
	if match == nil {
		return nil
	}
	var locales []string
	for _, raw := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(match[1], -1) {
		locales = append(locales, raw[1])
	}
	return locales
}

// pluralVariantKeys returns the catalog keys that carry a CLDR plural variant,
// which the translator reads as `<key>.<category>` (see web/src/i18n/translator.ts).
func (catalog *catalog) pluralVariantKeys() []string {
	categories := map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}
	var variants []string
	for _, key := range catalog.allKeys() {
		index := strings.LastIndex(key, ".")
		if index < 0 {
			continue
		}
		if categories[key[index+1:]] {
			variants = append(variants, key)
		}
	}
	sort.Strings(variants)
	return variants
}
