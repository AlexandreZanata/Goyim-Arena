package http

import (
	"html/template"
	"io"
)

const (
	verifySuccessTemplateSrc = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Email verificado — Goyim Arena</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem auto; max-width: 600px; padding: 0 1rem; line-height: 1.5; color: #111; }
		.card { border: 1px solid #e0e0e0; border-radius: 8px; padding: 2rem; }
		h1 { color: #15803d; font-size: 1.5rem; margin-top: 0; }
		a.button { display: inline-block; background-color: #111; color: #fff; text-decoration: none; padding: 0.6rem 1.2rem; border-radius: 4px; font-weight: bold; margin-top: 1rem; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Email verificado com sucesso</h1>
		<p>Sua conta no Goyim Arena foi ativada e seu endereço de email foi confirmado.</p>
		<p>Você já pode acessar a plataforma utilizando suas credenciais.</p>
		<a href="/login" class="button">Ir para o Login</a>
	</div>
</body>
</html>`

	verifyErrorTemplateSrc = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Falha na verificação — Goyim Arena</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem auto; max-width: 600px; padding: 0 1rem; line-height: 1.5; color: #111; }
		.card { border: 1px solid #fee2e2; background-color: #fef2f2; border-radius: 8px; padding: 2rem; }
		h1 { color: #b91c1c; font-size: 1.5rem; margin-top: 0; }
		a.button { display: inline-block; background-color: #111; color: #fff; text-decoration: none; padding: 0.6rem 1.2rem; border-radius: 4px; font-weight: bold; margin-top: 1rem; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Link de verificação inválido ou expirado</h1>
		<p>Não foi possível confirmar seu email. O link pode ter expirado ou já ter sido utilizado.</p>
		<p>Solicite um novo link de confirmação ou entre em contato com o suporte.</p>
		<a href="/login" class="button">Voltar ao início</a>
	</div>
</body>
</html>`

	passwordResetFormTemplateSrc = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Redefinir senha — Goyim Arena</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem auto; max-width: 500px; padding: 0 1rem; line-height: 1.5; color: #111; }
		.card { border: 1px solid #e0e0e0; border-radius: 8px; padding: 2rem; }
		h1 { font-size: 1.5rem; margin-top: 0; }
		.form-group { margin-bottom: 1.2rem; }
		label { display: block; font-weight: 600; margin-bottom: 0.3rem; }
		input[type="password"] { width: 100%; padding: 0.6rem; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box; font-size: 1rem; }
		button { background-color: #111; color: #fff; border: none; padding: 0.7rem 1.5rem; border-radius: 4px; font-size: 1rem; font-weight: bold; cursor: pointer; }
		button:hover { background-color: #333; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Redefinição de Senha</h1>
		<p>Informe sua nova senha para recuperar o acesso à sua conta.</p>
		<form method="POST" action="/api/v1/auth/password-reset/confirm">
			<input type="hidden" name="token" value="{{.Token}}">
			<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
			<div class="form-group">
				<label for="password">Nova Senha (mínimo 8 caracteres):</label>
				<input type="password" id="password" name="password" minlength="8" required autofocus>
			</div>
			<button type="submit">Salvar nova senha</button>
		</form>
	</div>
</body>
</html>`

	passwordResetSuccessTemplateSrc = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Senha alterada — Goyim Arena</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem auto; max-width: 600px; padding: 0 1rem; line-height: 1.5; color: #111; }
		.card { border: 1px solid #e0e0e0; border-radius: 8px; padding: 2rem; }
		h1 { color: #15803d; font-size: 1.5rem; margin-top: 0; }
		a.button { display: inline-block; background-color: #111; color: #fff; text-decoration: none; padding: 0.6rem 1.2rem; border-radius: 4px; font-weight: bold; margin-top: 1rem; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Senha redefinida com sucesso</h1>
		<p>Sua senha foi atualizada com sucesso e todas as sessões anteriores foram revogadas por segurança.</p>
		<p>Você já pode fazer login com sua nova credencial.</p>
		<a href="/login" class="button">Acessar minha conta</a>
	</div>
</body>
</html>`

	passwordResetErrorTemplateSrc = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Erro na redefinição — Goyim Arena</title>
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 2rem auto; max-width: 600px; padding: 0 1rem; line-height: 1.5; color: #111; }
		.card { border: 1px solid #fee2e2; background-color: #fef2f2; border-radius: 8px; padding: 2rem; }
		h1 { color: #b91c1c; font-size: 1.5rem; margin-top: 0; }
		a.button { display: inline-block; background-color: #111; color: #fff; text-decoration: none; padding: 0.6rem 1.2rem; border-radius: 4px; font-weight: bold; margin-top: 1rem; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Não foi possível redefinir sua senha</h1>
		<p>O token de recuperação é inválido, expirou ou já foi utilizado anteriormente.</p>
		<p>Solicite uma nova recuperação de senha para continuar.</p>
		<a href="/login" class="button">Voltar ao início</a>
	</div>
</body>
</html>`
)

// HTMLTemplates owns compiled templates for transactional email link landing pages (P04-T08).
type HTMLTemplates struct {
	verifySuccess        *template.Template
	verifyError          *template.Template
	passwordResetForm    *template.Template
	passwordResetSuccess *template.Template
	passwordResetError   *template.Template
}

// NewHTMLTemplates parses and compiles the minimal landing page HTML documents.
func NewHTMLTemplates() *HTMLTemplates {
	return &HTMLTemplates{
		verifySuccess:        template.Must(template.New("verifySuccess").Parse(verifySuccessTemplateSrc)),
		verifyError:          template.Must(template.New("verifyError").Parse(verifyErrorTemplateSrc)),
		passwordResetForm:    template.Must(template.New("passwordResetForm").Parse(passwordResetFormTemplateSrc)),
		passwordResetSuccess: template.Must(template.New("passwordResetSuccess").Parse(passwordResetSuccessTemplateSrc)),
		passwordResetError:   template.Must(template.New("passwordResetError").Parse(passwordResetErrorTemplateSrc)),
	}
}

// RenderVerifySuccess renders the email verification confirmation document.
func (t *HTMLTemplates) RenderVerifySuccess(w io.Writer) error {
	return t.verifySuccess.Execute(w, nil)
}

// RenderVerifyError renders the email verification failure document.
func (t *HTMLTemplates) RenderVerifyError(w io.Writer) error {
	return t.verifyError.Execute(w, nil)
}

// PasswordResetFormData parameters for rendering the reset form.
type PasswordResetFormData struct {
	Token     string
	CSRFToken string
}

// RenderPasswordResetForm renders the reset password interactive form.
func (t *HTMLTemplates) RenderPasswordResetForm(w io.Writer, data PasswordResetFormData) error {
	return t.passwordResetForm.Execute(w, data)
}

// RenderPasswordResetSuccess renders the password reset confirmation document.
func (t *HTMLTemplates) RenderPasswordResetSuccess(w io.Writer) error {
	return t.passwordResetSuccess.Execute(w, nil)
}

// RenderPasswordResetError renders the password reset error document.
func (t *HTMLTemplates) RenderPasswordResetError(w io.Writer) error {
	return t.passwordResetError.Execute(w, nil)
}
