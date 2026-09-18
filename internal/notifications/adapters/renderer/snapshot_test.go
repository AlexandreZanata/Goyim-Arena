package renderer_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexandreZanata/Goyim-Arena/internal/notifications/domain"
)

// snapshotName returns the committed artifact of one rendered email.
func snapshotName(templateID domain.TemplateID, locale domain.Locale) string {
	return filepath.Join("testdata", templateID.String()+"."+locale.String()+".golden")
}

// snapshot is the committed representation of one rendered email: the subject
// and both representations, so a change to any of the three is a reviewable
// diff instead of a silent edit to what a recipient reads.
func snapshot(body domain.Body) string {
	var builder strings.Builder
	builder.WriteString("subject: " + body.Subject + "\n")
	builder.WriteString("--- text\n")
	builder.WriteString(body.Text + "\n")
	builder.WriteString("--- html\n")
	builder.WriteString(body.HTML)
	return builder.String()
}

// TestRenderedEmailsMatchTheCommittedSnapshots is the guard the standard asks
// for: one committed artifact per template and locale, compared byte for byte.
//
// The comparison never rewrites the artifact. A mismatch means the email a
// recipient reads has changed, and the fix is a deliberate edit to the file in
// the same commit — reviewed as the change in wording or markup that it is.
func TestRenderedEmailsMatchTheCommittedSnapshots(t *testing.T) {
	engine := newRenderer(t)
	const name, code = "Ana", "K7QP-2M4Z-9RTX"
	for _, templateID := range domain.TemplateIDs() {
		for _, locale := range domain.Locales() {
			t.Run(templateID.String()+"."+locale.String(), func(t *testing.T) {
				body, err := engine.Render(templateID, locale, values(t, name, code))
				if err != nil {
					t.Fatalf("Render() error = %v", err)
				}
				// Subject, text and html exist for every template in every
				// shipped locale: the snapshot is incomplete without all
				// three.
				if body.Subject == "" || body.Text == "" || body.HTML == "" {
					t.Fatalf("rendered email is incomplete: %+v", body)
				}
				path := snapshotName(templateID, locale)
				committed, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				if rendered := snapshot(body); rendered != string(committed) {
					t.Errorf("rendered email differs from %s.\n--- committed\n%s\n--- rendered\n%s\n"+
						"Update the artifact deliberately if the change is intended.", path, committed, rendered)
				}
			})
		}
	}
}

// TestEveryTemplateIsReallyLocalized closes the other half: the artifacts
// existing is not the same as being localized, and a hardcoded string would
// pass the snapshot check for exactly one locale.
func TestEveryTemplateIsReallyLocalized(t *testing.T) {
	engine := newRenderer(t)
	for _, templateID := range domain.TemplateIDs() {
		rendered := make(map[domain.Locale]domain.Body, len(domain.Locales()))
		for _, locale := range domain.Locales() {
			body, err := engine.Render(templateID, locale, values(t, "Ana", "K7QP-2M4Z-9RTX"))
			if err != nil {
				t.Fatalf("Render(%s, %s) error = %v", templateID, locale, err)
			}
			rendered[locale] = body
		}
		left, right := rendered[domain.LocaleBrazilianPortuguese], rendered[domain.LocaleAmericanEnglish]
		for _, part := range []struct {
			name string
			pt   string
			en   string
		}{
			{"subject", left.Subject, right.Subject},
			{"text", left.Text, right.Text},
			{"html", left.HTML, right.HTML},
		} {
			if part.pt == "" || part.en == "" {
				t.Errorf("%s: %s is empty in one of the locales", templateID, part.name)
				continue
			}
			if part.pt == part.en {
				t.Errorf("%s: %s is identical in both locales, so it is not localized", templateID, part.name)
			}
		}
	}
}

// tagPattern matches one markup tag, attributes included.
var tagPattern = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

// TestTemplatesShareOneDocumentStructure is the parity of the markup: the
// templates differ in the catalog block they read, never in the document they
// render. A template that silently gained or lost markup — or that reached for
// a second document — fails here, and the values it fills are proven to be the
// only difference.
func TestTemplatesShareOneDocumentStructure(t *testing.T) {
	engine := newRenderer(t)
	ids := domain.TemplateIDs()
	if len(ids) < 2 {
		t.Fatalf("templates = %d, want at least two to compare", len(ids))
	}
	for _, locale := range domain.Locales() {
		t.Run(locale.String(), func(t *testing.T) {
			markup := make(map[domain.TemplateID]string, len(ids))
			for _, templateID := range ids {
				body, err := engine.Render(templateID, locale, values(t, "Ana", "K7QP-2M4Z-9RTX"))
				if err != nil {
					t.Fatalf("Render(%s, %s) error = %v", templateID, locale, err)
				}
				markup[templateID] = body.HTML
			}
			first := tagPattern.FindAllString(markup[ids[0]], -1)
			second := tagPattern.FindAllString(markup[ids[1]], -1)
			if strings.Join(first, "|") != strings.Join(second, "|") {
				t.Errorf("markup differs between templates:\n%s\n%s", strings.Join(first, "\n"), strings.Join(second, "\n"))
			}
			valuesFirst := strings.Join(tagPattern.Split(markup[ids[0]], -1), "|")
			valuesSecond := strings.Join(tagPattern.Split(markup[ids[1]], -1), "|")
			if valuesFirst == valuesSecond {
				t.Error("both templates filled the document with the same values")
			}
		})
	}
}
