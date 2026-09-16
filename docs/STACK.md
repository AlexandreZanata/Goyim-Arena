# Stack tecnológica

**Status:** aprovada para o MVP

**Última revisão:** 2026-09-16

## 1. Resumo

Goyim Arena será um monólito modular, renderizado no servidor e operado inicialmente em uma VPS de 16 GB. A stack prioriza baixo custo fixo, auditabilidade, páginas públicas rápidas e uma rota clara de crescimento.

- **Borda:** Cloudflare DNS, CDN, WAF e Turnstile.
- **Proxy e TLS:** Caddy 2.
- **Aplicação:** Go 1.27.x, usando sempre o patch suportado mais recente.
- **Roteamento HTTP:** chi.
- **HTML:** templ.
- **Interatividade:** HTMX 2.x e JavaScript vanilla pontual.
- **CSS:** Tailwind CSS 4.x.
- **Banco:** PostgreSQL 18.x, usando sempre o patch suportado mais recente.
- **Driver:** pgx v5.
- **SQL tipado:** sqlc.
- **Migrations:** goose, com migrations SQL versionadas.
- **Sessões:** tokens opacos em cookie e armazenamento PostgreSQL, com SCS v2.
- **Senhas:** Argon2id via `golang.org/x/crypto`.
- **Busca inicial:** full-text search e índices GIN do PostgreSQL.
- **Jobs:** tabela PostgreSQL e worker Go com `FOR UPDATE SKIP LOCKED`.
- **Pagamento:** Stripe Checkout, Billing e webhooks.
- **Email:** Resend.
- **Storage futuro:** Cloudflare R2 quando arquivos forem realmente necessários.
- **Analytics:** PostHog com eventos mínimos e sem conteúdo privado.
- **Erros:** Sentry.
- **Logs:** `log/slog` em JSON para stdout.
- **Deploy:** Docker Compose em Debian estável ou Ubuntu LTS.
- **CI/CD:** GitHub Actions.
- **Testes de carga:** k6.
- **E2E:** Playwright.

## 2. Política de versões

Documentos registram linhas suportadas, não imagens flutuantes. Builds e deploys devem fixar patch e digest quando houver implementação.

- Go: linha 1.27, patch mais recente testado; atualmente 1.27.1.
- PostgreSQL: linha 18, patch mais recente testado; atualmente 18.6.
- Dependências Go: versões fixadas por `go.mod` e `go.sum`.
- Dependências de frontend: versões fixadas e artefatos servidos pela própria aplicação; produção não depende de CDN de terceiros para HTMX.
- Imagens de container: tags semânticas e digest no ambiente de produção.
- Atualizações de segurança: prioridade, com rollback documentado.

Não atualizar versões automaticamente em produção sem CI, backup válido e teste de migração.

## 3. Escolhas deliberadas

### Go em vez de Node SSR

O produto é predominantemente HTTP, validação, SQL e HTML. Go oferece processo único, concorrência simples, consumo previsível e deploy por binário/container. A escolha sacrifica parte do ecossistema visual de React em troca de operação mais simples.

### HTML server-side em vez de SPA

Páginas públicas precisam de SEO, baixo JavaScript e cache eficiente. templ mantém componentes tipados; HTMX atualiza fragmentos sem transformar toda a aplicação em cliente stateful.

### JavaScript vanilla antes de Alpine.js

Alpine.js não entra como dependência padrão. Será reconsiderado apenas se houver estado de interface repetitivo que HTMX e JavaScript pequeno não resolvam claramente.

### SQL explícito em vez de ORM

pgx e sqlc preservam SQL visível, geram tipos e permitem analisar queries críticas. Migrations continuam sendo SQL legível.

### PostgreSQL antes de novos serviços

PostgreSQL atende persistência, transações, sessões, busca, jobs e projeções iniciais. Redis, Valkey, RabbitMQ, Kafka, Elasticsearch e banco vetorial só entram após gargalo medido e ADR específico.

## 4. Ferramentas de desenvolvimento

- `go test`, race detector, fuzzing dirigido e benchmarks.
- `golangci-lint` com configuração versionada.
- `govulncheck` para vulnerabilidades do ecossistema Go.
- `sqlc vet` e validação das migrations em banco descartável.
- Testcontainers-Go para integrações que exigem PostgreSQL real.
- Playwright para fluxos críticos no browser.
- k6 para capacidade e regressões de desempenho.
- Gitleaks e Dependabot para segredos e dependências.

## 5. O que não entra no MVP

- React, Next.js ou SPA;
- Supabase;
- Redis ou Valkey;
- Kubernetes;
- microserviços;
- RabbitMQ ou Kafka;
- Elasticsearch, Meilisearch ou Typesense;
- blockchain ou banco vetorial;
- IA no fluxo central;
- armazenamento de uploads no disco local da VPS.

## 6. Regra de expansão

> Nenhuma nova infraestrutura entra enquanto Go, PostgreSQL e Cloudflare resolverem o problema de forma razoável e mensurável.

Uma exceção precisa declarar problema, evidência, custo esperado, alternativa simples, plano de saída e novo ADR.

## 7. Referências oficiais

- [Histórico de releases do Go](https://go.dev/doc/devel/release)
- [Release notes do PostgreSQL](https://www.postgresql.org/docs/release/)
- [Documentação do templ](https://templ.guide/)
- [Documentação do HTMX](https://htmx.org/docs/)
- [Documentação do Caddy](https://caddyserver.com/docs/)
- [Cache do Cloudflare](https://developers.cloudflare.com/cache/)
