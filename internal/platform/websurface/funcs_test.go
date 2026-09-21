// Tests of the shared template functions (P18-T10): the direction of a
// document is rendered from the same expression as its language, and a
// surface that declares its own function keeps it.
package websurface

import (
	"html/template"
	"strings"
	"testing"
)

// renderDocument renders the head every surface shares (`lang` and `dir` from
// one expression) so the function map is exercised where it is used.
func renderDocument(t *testing.T, funcs template.FuncMap, lang string) string {
	t.Helper()

	document, err := template.New("probe").Funcs(funcs).Parse(`<html lang="{{.Lang}}" dir="{{dir .Lang}}"></html>`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var rendered strings.Builder
	if err := document.Execute(&rendered, map[string]string{"Lang": lang}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return rendered.String()
}

func TestFuncsRenderTheDirectionOfTheDeclaredLocale(t *testing.T) {
	t.Parallel()

	cases := map[string]string{"pt-BR": "ltr", "en-US": "ltr", "ar": "rtl", "he-IL": "rtl"}
	for locale, direction := range cases {
		want := `<html lang="` + locale + `" dir="` + direction + `"></html>`
		if got := renderDocument(t, Funcs(), locale); got != want {
			t.Errorf("Funcs() rendered %s, want %s", got, want)
		}
	}
}

// TestWithFuncsKeepsBothMaps covers the composition: the functions a surface
// declares stay available, and the shared `dir` stays with it.
func TestWithFuncsKeepsBothMaps(t *testing.T) {
	t.Parallel()

	declared := template.FuncMap{
		"asset": func(name string) string { return "/assets/" + name },
	}
	composed := WithFuncs(declared)

	for _, name := range []string{"dir", "asset"} {
		if _, ok := composed[name]; !ok {
			t.Errorf("WithFuncs dropped %q", name)
		}
	}
	if got := renderDocument(t, composed, "ar"); !strings.Contains(got, `dir="rtl"`) {
		t.Errorf("the composed map lost the direction: %s", got)
	}
}

// TestWithFuncsLetsASurfaceOverride keeps the exception visible: a document
// that needs a different direction says so where it parses its templates.
func TestWithFuncsLetsASurfaceOverride(t *testing.T) {
	t.Parallel()

	composed := WithFuncs(template.FuncMap{"dir": func(string) string { return "rtl" }})
	if got := renderDocument(t, composed, "pt-BR"); !strings.Contains(got, `dir="rtl"`) {
		t.Errorf("the declared function did not win: %s", got)
	}
}
