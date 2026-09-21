// Package localized is the positive fixture of the localization scanner: the
// same document, localized. Every sentence arrives from the catalog as an
// action and the head declares the locale and its direction.
package localized

// localizedDocument is the shape the rules must accept.
const localizedDocument = `<!DOCTYPE html>
<html lang="{{.Lang}}" dir="{{dir .Lang}}">
<head>
<title>{{.PageTitle}}</title>
<style>
body { margin-inline: 2rem; }
</style>
</head>
<body>
<h1>{{.Heading}}</h1>
<p>{{.Detail}}</p>
<form method="POST" action="{{.Form.Action}}">
<label for="password">{{.Form.Label}}</label>
<input type="password" id="password" name="password" minlength="8" required>
<button type="submit">{{.Form.Submit}}</button>
</form>
</body>
</html>`
