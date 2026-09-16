# Segurança técnica

**Status:** requisitos para implementação

**Referência geral:** OWASP ASVS 5

## 1. Modelo de confiança

- Cloudflare reduz tráfego malicioso, mas não é fronteira única de autorização.
- Caddy encaminha requisições; a aplicação valida identidade, autorização e entrada.
- PostgreSQL não é acessível pela Internet.
- Stripe é autoridade do pagamento; o ledger local é autoridade do uso de INK.
- Nenhum dado vindo de browser, webhook, header de proxy ou job é confiável sem validação.

## 2. Autenticação

- Sessão com token aleatório de alta entropia enviado uma vez ao browser.
- Apenas hash do token é persistido.
- Cookie `HttpOnly`, `Secure`, `SameSite=Lax`, com escopo mínimo.
- Rotação após login, elevação de privilégio e alteração de credencial.
- Expiração absoluta e por inatividade; encerramento de outras sessões disponível.
- Senhas com Argon2id e parâmetros versionados, ajustados por benchmark.
- Tokens de confirmação e recuperação são de uso único, curtos e persistidos como hash.
- Administradores exigem segundo fator antes do beta público.
- Respostas de login e recuperação não revelam se um email existe.

Não implementar JWT como sessão principal no browser. JWT pode ser reavaliado para integração específica, não por conveniência.

## 3. Autorização

- Toda ação possui verificação server-side de ator, recurso e permissão.
- IDs enviados pelo cliente nunca provam ownership.
- Rotas administrativas usam namespace, middleware e auditoria próprios.
- Contas pagantes não recebem bypass de regras.
- Acesso direto ao banco usa papel sem permissão de migration ou superuser.

## 4. Aplicação web

- HTML inicial usa `html/template` com escaping padrão; HTML arbitrário de usuário é proibido.
- Componentes TypeScript usam DOM seguro e `textContent`; `innerHTML` é proibido para dados dinâmicos.
- Conteúdo do MVP é plaintext. Markdown, se introduzido, exige parser com allowlist e sanitização.
- CSRF token em toda mutação baseada em cookie, além de validação de origem quando aplicável.
- CSP restritiva; módulos e estilos próprios, sem `unsafe-inline` ou `unsafe-eval` por padrão.
- Headers: HSTS, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy` e proteção de framing.
- Uploads não entram no MVP. Quando entrarem, usar object storage, tipo detectado, limites e domínio de entrega separado quando necessário.
- Redirecionamentos e URLs externas passam por validação.

## 5. Cache

- Rotas privadas sempre enviam `Cache-Control: private, no-store`.
- Respostas com cookie ou identidade não podem ser armazenadas no edge.
- Cache público não inclui posição individual, saldo, email ou informação administrativa.
- Regras Cloudflare têm testes para visitante, autenticado e conteúdo removido.
- Cache poisoning e cache deception fazem parte dos testes de segurança.

## 6. Abuso e rate limiting

Camadas complementares:

- Cloudflare WAF e limites de borda;
- Turnstile em cadastro, recuperação, publicação de Arena e risco elevado;
- limites no Go por conta, sessão, ação e sinais de rede;
- constraints e idempotência no PostgreSQL;
- detecção e revisão de padrões de Sybil, farming, brigading e reciprocidade.

IP é sinal imperfeito e dado pessoal potencial. Nunca é prova isolada de abuso. A aplicação só confia em headers de IP recebidos de proxies explicitamente confiáveis.

## 7. Wallet e concorrência

- Ledger append-only; saldo materializado nunca é autoridade isolada.
- Débito e publicação de argumento na mesma transação.
- Consumo de passe e publicação de Arena na mesma transação.
- Lock e constraints impedem double spend.
- Toda entrada externa repetível possui chave de idempotência.
- Ajustes administrativos exigem motivo, ator e trilha de auditoria.

## 8. Stripe

- Verificar assinatura usando corpo bruto e secret correto do ambiente.
- Persistir `stripe_event_id` único antes de aplicar efeito.
- Não creditar por visita à página de sucesso.
- Processamento idempotente e transacional.
- Não registrar payload integral nem dados desnecessários.
- Reembolsos e chargebacks possuem estados explícitos; nunca fazem mutação silenciosa do ledger.

## 9. Segredos e supply chain

- Nenhum segredo no Git, imagem ou frontend.
- Arquivo de ambiente de produção fora do checkout, owner root e modo `0600`, ou solução equivalente.
- GitHub Actions usa permissões mínimas e OIDC quando o fornecedor suportar.
- Actions de terceiros fixadas por commit SHA.
- Dependabot, `govulncheck`, Gitleaks e scanner de imagem no CI.
- SBOM e provenance entram antes da primeira release pública paga.

## 10. Logs e auditoria

- Logs estruturados possuem request ID, classe de evento e identificadores não sensíveis.
- Nunca registrar senha, token, cookie, segredo, texto de denúncia, corpo integral de webhook ou rascunho.
- Ações administrativas e de moderação entram em trilha separada e imutável por regra de aplicação.
- Sentry recebe redaction antes do envio.

## 11. Checklist de release

- threat model revisado;
- testes de autorização e CSRF;
- testes de cache público/privado;
- teste de webhook repetido;
- teste de double spend e corrida;
- restauração de backup comprovada;
- secrets scan limpo;
- dependências sem vulnerabilidade crítica conhecida;
- conta admin com MFA;
- origem e PostgreSQL não expostos;
- contato privado de segurança disponível.

## 12. Reporte de vulnerabilidade

Não publicar vulnerabilidade explorável em issue. Usar o canal privado de segurança do GitHub definido no arquivo `SECURITY.md` da raiz. Detalhes podem ser publicados após correção coordenada.
