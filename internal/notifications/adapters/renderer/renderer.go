// Package renderer composes the localized transactional emails of Goyim
// Arena from the shared catalog (P15-T03).
//
// Two rules shape the package. First, every localized string comes from
// internal/i18n, so an email and the interface it mirrors never drift
// apart. Second, the markup is produced by html/template from a fixed
// template per message type — never by concatenating localized strings with
// user data. The distinction matters: the catalog string is substituted
// verbatim (i18n documents that escaping belongs to the rendering context),
// so a user-supplied display name reaches the document only as an
// html/template action, where the correct escaping for that position is
// applied by construction. No code path here can produce HTML that skipped
// it.
package renderer

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/AlexandreZanata/Goyim-Arena/internal/i18n"
	"github.com/AlexandreZanata/Goyim-Arena/internal/notifications/domain"
)

// Catalog keys, one block per message type, shared by the plain-text and
// HTML representations. The message-type segment is the template
// identifier, so a new template means a new catalog block and nothing else.
const (
	keyGreeting  = "email.greeting"
	keySignature = "email.signature"
	keySubject   = "email.%s.subject"
	keyLead      = "email.%s.lead"
	keyCodeLabel = "email.%s.code_label"
)

// HTML sources. A template is identified by message type and lives here as
// a constant: it is code, reviewed and versioned, and it is the only
// document the package can emit.
const htmlVerification = `<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<title>{{.Subject}}</title>
</head>
<body>
<p>{{.Greeting}} {{.Name}}</p>
<p>{{.Lead}}</p>
<p><strong>{{.CodeLabel}}</strong></p>
<p>{{.Code}}</p>
<p>{{.Signature}}</p>
</body>
</html>
`

const htmlPasswordReset = `<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<title>{{.Subject}}</title>
</head>
<body>
<p>{{.Greeting}} {{.Name}}</p>
<p>{{.Lead}}</p>
<p><strong>{{.CodeLabel}}</strong></p>
<p>{{.Code}}</p>
<p>{{.Signature}}</p>
</body>
</html>
`

// htmlData is the data handed to the templates. Greeting, Lead, CodeLabel,
// Signature and Subject are catalog strings (no caller input); Name and Code
// are raw values that html/template escapes for the position they occupy.
type htmlData struct {
	Language  string
	Subject   string
	Greeting  string
	Name      string
	Lead      string
	CodeLabel string
	Code      string
	Signature string
}

// Renderer composes bodies from the catalog and the fixed HTML templates.
// It is immutable after construction and safe for concurrent use.
type Renderer struct {
	templates map[domain.TemplateID]*template.Template
}

// NewRenderer builds the renderer. It fails if a template does not parse —
// a programming error that must surface at wiring time, never as a broken
// email — or if a message type has no source.
func NewRenderer() (*Renderer, error) {
	sources := map[domain.TemplateID]string{
		domain.TemplateVerification:  htmlVerification,
		domain.TemplatePasswordReset: htmlPasswordReset,
	}
	templates := make(map[domain.TemplateID]*template.Template, len(sources))
	for _, id := range domain.TemplateIDs() {
		source, ok := sources[id]
		if !ok {
			return nil, fmt.Errorf("renderer: no html source for template %q", id)
		}
		parsed, err := template.New(id.String()).Parse(source)
		if err != nil {
			return nil, fmt.Errorf("renderer: parse template %q: %w", id, err)
		}
		templates[id] = parsed
	}
	return &Renderer{templates: templates}, nil
}

// Render composes the body for one message. Unknown templates and locales
// are refused before any lookup, and the values are re-validated here so the
// adapter does not depend on the caller having done it.
func (r *Renderer) Render(templateID domain.TemplateID, locale domain.Locale, values domain.TemplateValues) (domain.Body, error) {
	if r == nil {
		return domain.Body{}, domain.ErrMissingDependency
	}
	if !templateID.Valid() {
		return domain.Body{}, domain.ErrUnsupportedTemplate
	}
	if !locale.Valid() {
		return domain.Body{}, domain.ErrUnsupportedLocale
	}
	if _, err := domain.NewTemplateValues(values.Name, values.Code); err != nil {
		return domain.Body{}, err
	}
	source, ok := r.templates[templateID]
	if !ok {
		return domain.Body{}, domain.ErrUnsupportedTemplate
	}
	namespace := templateID.String()
	subject, err := message(locale, fmt.Sprintf(keySubject, namespace))
	if err != nil {
		return domain.Body{}, err
	}
	greeting, err := message(locale, keyGreeting)
	if err != nil {
		return domain.Body{}, err
	}
	lead, err := message(locale, fmt.Sprintf(keyLead, namespace))
	if err != nil {
		return domain.Body{}, err
	}
	codeLabel, err := message(locale, fmt.Sprintf(keyCodeLabel, namespace))
	if err != nil {
		return domain.Body{}, err
	}
	signature, err := message(locale, keySignature)
	if err != nil {
		return domain.Body{}, err
	}

	var html bytes.Buffer
	// html/template applies the escaping of the surrounding context, which
	// is why the user-supplied fields are passed as data rather than
	// interpolated into the localized strings first.
	if err := source.Execute(&html, htmlData{
		Language:  locale.String(),
		Subject:   subject,
		Greeting:  greeting,
		Name:      values.Name,
		Lead:      lead,
		CodeLabel: codeLabel,
		Code:      values.Code,
		Signature: signature,
	}); err != nil {
		return domain.Body{}, fmt.Errorf("renderer: execute template %q: %w", templateID, err)
	}

	return domain.NewBody(
		subject,
		plainText(greeting, values.Name, lead, codeLabel, values.Code, signature),
		html.String(),
	)
}

// message resolves one catalog key. A miss is a programming error: the
// catalog is generated, so a missing key means the template and the
// catalog disagree.
func message(locale domain.Locale, key string) (string, error) {
	resolved, err := i18n.Format(locale.String(), key, nil)
	if err != nil {
		return "", fmt.Errorf("renderer: resolve %q in %q: %w", key, locale, err)
	}
	return resolved, nil
}

// plainText builds the text/plain alternative. Substitution is verbatim
// here on purpose: this representation carries no markup, so there is no
// context to escape for.
func plainText(greeting, name, lead, codeLabel, code, signature string) string {
	var builder strings.Builder
	builder.WriteString(greeting)
	if name != "" {
		builder.WriteString(" ")
		builder.WriteString(name)
	}
	builder.WriteString("\n\n")
	builder.WriteString(lead)
	builder.WriteString("\n\n")
	builder.WriteString(codeLabel)
	builder.WriteString(": ")
	builder.WriteString(code)
	builder.WriteString("\n\n")
	builder.WriteString(signature)
	return builder.String()
}
