// Tests of the premise the policy rests on (P16-T01): no server-rendered
// page carries executable inline code. That is what makes `script-src 'self'`
// and `style-src 'self'` sufficient without a nonce — and a nonce is not an
// option here, because a per-request value inside the body of a public,
// cacheable document moves its ETag on every request.
//
// The premise is asserted against the real templates of both server-rendered
// surfaces with adversarial values, so adding an inline event handler or an
// executable inline script fails the build instead of quietly producing pages
// that the policy blocks in the browser.
package securityheaders_test

import (
	"bytes"
	"encoding/xml"
	"html/template"
	"io"
	"strings"
	"testing"

	arenashtml "github.com/AlexandreZanata/Goyim-Arena/internal/arenas/adapters/html"
	transparencyhttp "github.com/AlexandreZanata/Goyim-Arena/internal/transparency/adapters/http"
)

// dataBlockType is the only script type a server-rendered page may carry. The
// HTML standard never prepares a script element whose type is not a
// JavaScript MIME type for execution — it is a data block — so script-src
// does not govern it and the JSON-LD payload survives a policy without
// 'unsafe-inline'.
const dataBlockType = "application/ld+json"

// hostileValue is a value no template may place anywhere the browser would
// execute: it carries an element, a scheme and two handlers.
const hostileValue = `<script>alert(1)</script> javascript:alert(2) onerror=alert(3) " ' &`

// scanPage walks one rendered document and fails on anything that would need
// a widened policy. It returns the number of script data blocks and the number
// of elements it read, so a caller can prove the scan was not vacuous.
func scanPage(t *testing.T, surface, document string) (dataBlocks, elements int) {
	t.Helper()

	decoder := xml.NewDecoder(strings.NewReader(document))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: the rendered document is not well-formed markup, so it cannot be scanned: %v", surface, err)
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		elements++

		switch strings.ToLower(start.Name.Local) {
		case "style":
			t.Errorf("%s: an inline <style> element would require style-src 'unsafe-inline'", surface)
		case "script":
			if scriptType := attribute(start, "type"); !strings.EqualFold(scriptType, dataBlockType) {
				t.Errorf("%s: inline <script type=%q> is executable; the policy has no 'unsafe-inline' and no nonce", surface, scriptType)
			} else {
				dataBlocks++
			}
		}

		for _, attributeValue := range start.Attr {
			name := strings.ToLower(attributeValue.Name.Local)
			value := strings.ToLower(strings.TrimSpace(attributeValue.Value))
			switch {
			case name == "style":
				t.Errorf("%s: inline style attribute %q would require style-src 'unsafe-inline'", surface, attributeValue.Value)
			case len(name) > 2 && strings.HasPrefix(name, "on"):
				t.Errorf("%s: inline event handler %s would require script-src 'unsafe-inline'", surface, name)
			case strings.HasPrefix(value, "javascript:"), strings.HasPrefix(value, "vbscript:"), strings.HasPrefix(value, "data:"):
				t.Errorf("%s: attribute %s carries a scheme the policy does not allow: %q", surface, name, attributeValue.Value)
			}
		}
	}
	return dataBlocks, elements
}

// attribute returns the value of one attribute, or the empty string.
func attribute(element xml.StartElement, name string) string {
	for _, attribute := range element.Attr {
		if strings.EqualFold(attribute.Name.Local, name) {
			return attribute.Value
		}
	}
	return ""
}

// TestArenaDocumentCarriesOnlyItsJSONLDDataBlock scans the cacheable public
// Arena document and its error pages.
func TestArenaDocumentCarriesOnlyItsJSONLDDataBlock(t *testing.T) {
	t.Parallel()

	templates := arenashtml.NewTemplates()

	var document bytes.Buffer
	err := templates.RenderDocument(&document, arenashtml.DocumentData{
		Lang:        "pt-BR",
		PageTitle:   hostileValue,
		Statement:   hostileValue,
		Context:     hostileValue,
		Description: hostileValue,
		Canonical:   "https://goyimarena.example/d/exemplo",
		OGLocale:    "pt_BR",
		StatusLabel: hostileValue,
		PublishedAt: "2026-09-18T12:00:00Z",
		JSONLD:      template.JS(`{"@context":"https://schema.org","@type":"Article"}`),
	})
	if err != nil {
		t.Fatalf("RenderDocument() error = %v", err)
	}

	rendered := document.String()
	if !strings.Contains(rendered, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("the hostile value was not rendered escaped, so the scan is not reading the real document:\n%s", rendered)
	}
	blocks, elements := scanPage(t, "arena document", rendered)
	if blocks != 1 {
		t.Errorf("arena document carries %d script data blocks, want exactly the JSON-LD one", blocks)
	}
	if elements < 5 {
		t.Errorf("arena document scan read %d elements; it is not reading the document", elements)
	}

	for name, errorData := range map[string]arenashtml.ErrorData{
		"not found": {
			Lang:      "pt-BR",
			PageTitle: hostileValue,
			Title:     hostileValue,
			Detail:    hostileValue,
		},
		"gone": {
			Lang:      "en-US",
			PageTitle: hostileValue,
			Title:     hostileValue,
			Detail:    hostileValue,
		},
	} {
		var page bytes.Buffer
		if err := templates.RenderError(&page, errorData); err != nil {
			t.Fatalf("RenderError(%s) error = %v", name, err)
		}
		if !strings.Contains(page.String(), "&lt;script&gt;alert(1)&lt;/script&gt;") {
			t.Fatalf("arena %s page did not render the hostile value escaped:\n%s", name, page.String())
		}
		blocks, elements := scanPage(t, "arena "+name+" page", page.String())
		if blocks != 0 {
			t.Errorf("arena %s page carries %d script data blocks, want none", name, blocks)
		}
		if elements < 3 {
			t.Errorf("arena %s page scan read %d elements; it is not reading the document", name, elements)
		}
	}
}

// TestTransparencyDocumentCarriesNoScriptAtAll scans the public transparency
// document, the other server-rendered surface, and proves the scan reads it.
func TestTransparencyDocumentCarriesNoScriptAtAll(t *testing.T) {
	t.Parallel()

	templates, err := transparencyhttp.NewTemplates()
	if err != nil {
		t.Fatalf("NewTemplates() error = %v", err)
	}

	var document bytes.Buffer
	err = templates.RenderDocument(&document, transparencyhttp.TransparencyDocument{
		Lang:         "pt-BR",
		PageTitle:    hostileValue,
		Heading:      hostileValue,
		Period:       hostileValue,
		Updated:      hostileValue,
		Methodology:  hostileValue,
		MetricHeader: hostileValue,
		ValueHeader:  hostileValue,
		Rows: []transparencyhttp.TransparencyRow{
			{Code: "arenas_published", Value: 6},
			{Code: "ink_free_granted", Value: 120},
		},
	})
	if err != nil {
		t.Fatalf("RenderDocument() error = %v", err)
	}

	rendered := document.String()
	if !strings.Contains(rendered, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("the hostile value was not rendered escaped, so the scan is not reading the real document:\n%s", rendered)
	}
	if !strings.Contains(rendered, "<td>arenas_published</td>") {
		t.Fatalf("the document lost its metric rows:\n%s", rendered)
	}

	blocks, elements := scanPage(t, "transparency document", rendered)
	if blocks != 0 {
		t.Errorf("transparency document carries %d script data blocks, want none", blocks)
	}
	if elements < 8 {
		t.Errorf("transparency document scan read %d elements; it is not reading the document", elements)
	}
}
