# Política de dependências

**Status:** padrão obrigatório

**Referência arquitetural:** [ADR-011](adr/ADR-011-dependency-admission-policy.md) · [STACK.md](STACK.md) · [ARCHITECTURE.md](ARCHITECTURE.md)

**Última revisão:** 2026-09-17

---

## 1. Princípio fundamental: Standard Library First

O Goyim Arena adota o princípio de prioridade máxima às capacidades nativas das plataformas oficiais:
1. **No backend:** a biblioteca padrão da linguagem Go (`net/http`, `crypto`, `database/sql`, `html/template`, `log/slog`, etc.) é a primeira e principal escolha para resolução de problemas técnicos e arquiteturais.
2. **No frontend:** as APIs e padrões nativos da plataforma web (HTML semântico, Custom Elements/Web Components, CSS moderno com custom properties e `@layer`, `fetch`, `AbortController`) constituem o alicerce exclusivo de execução.

Dependências de terceiros não são conveniências para evitar código simples; são passivos de longo prazo que introduzem riscos de segurança, manutenção e obsolescência. Nenhuma dependência externa é admitida sem que a alternativa nativa tenha sido avaliada e sua insuficiência comprovada.

---

## 2. Política para o frontend

### 2.1 Zero runtime externo no browser
- O navegador do usuário **não executa nenhuma biblioteca de terceiros**. É estritamente proibido o uso de frameworks (como React, Vue, Angular, Svelte), microframeworks (como HTMX, Alpine.js), bibliotecas utilitárias (como jQuery, Lodash, Axios) ou bibliotecas de componentes.
- Componentes de interface são construídos exclusivamente como Custom Elements nativos com o prefixo `ga-` (conforme documentado em [FRONTEND.md](FRONTEND.md)).
- Estilização utiliza CSS nativo com `@layer`, container queries e custom properties. Proibido Tailwind, Bootstrap, Sass ou soluções de CSS-in-JS.
- Proibida a importação de scripts ou estilos a partir de CDNs externas em tempo de execução.

### 2.2 Dependências de build do frontend
- A única dependência autorizada para o frontend é o compilador oficial `typescript`, fixado e executado exclusivamente durante o processo de build para emissão de JavaScript ESM nativo.
- Proibido o uso de bundlers complexos (Vite, Webpack, Rollup, Babel). A compilação é realizada diretamente pelo `tsc`.
- Ferramentas de teste E2E (como Playwright) são confinadas ao pipeline de testes e nunca incluídas no pacote de produção. O pacote isolado é `tools/e2e` (P18-T07): ele depende de `web/`, nunca o contrário, e `tools/e2e/isolation-check.sh` recusa o build se o runner aparecer no pacote do frontend, no build referenciado pelas páginas ou no binário.

---

## 3. Política para o backend

### 3.1 Go puro no domínio e na aplicação
- As camadas centrais (`domain` e `application` em `internal/<modulo>/`) utilizam exclusivamente a biblioteca padrão do Go.
- É estritamente proibido importar drivers de banco de dados, bibliotecas de serialização de terceiros, frameworks HTTP, SDKs de fornecedores ou utilitários externos no domínio e nos casos de uso.

### 3.2 Isolamento estrito por Adapter
- Toda dependência externa homologada deve ser confinada a um **Adapter Outbound** ou **Adapter Inbound** específico.
- Tipos concretos de fornecedores (como structs do SDK Stripe, tipos do driver `pgx` ou handlers do Sentry) **nunca** ultrapassam a fronteira do adapter.
- A camada de aplicação define portas (ports / interfaces) pequenas e coesas; o adapter implementa essas interfaces traduzindo tipos externos para entidades ou value objects do domínio.

---

## 4. Catálogo de dependências homologadas

Cada dependência admitida no projeto possui uma classe clara, um owner responsável e um escopo restrito de atuação:

| Dependência / Tecnologia | Classe | Owner / Escopo | Finalidade | Licença |
|---|---|---|---|---|
| `typescript` (oficial) | Dev / Build Tooling | `web/` | Compilação estrita de TypeScript para módulos ESM nativos | Apache-2.0 |
| `github.com/jackc/pgx/v5` | Runtime Backend (Adapter) | `internal/platform/adapters/out/postgres` | Driver PostgreSQL de alta performance e conexão com banco | MIT |
| `sqlc` | Dev / Codegen Tooling | `db/queries` e `adapters/out/postgres` | Compilação de consultas SQL tipadas para Go sem reflexão | MIT |
| `goose` (`github.com/pressly/goose/v3`) | Dev / Migration Tooling | `db/migrations` | Gerenciamento e execução de migrations versionadas em SQL | Apache-2.0 |
| `golang.org/x/crypto` | Runtime Backend (Adapter / Platform) | `internal/platform/crypto` | Hashing seguro de senhas com algoritmo Argon2id | BSD-3-Clause |
| `github.com/stripe/stripe-go` | Runtime Backend (Adapter) | `internal/billing/adapters/out/stripe` | Integração de Checkout, Billing e validação de webhooks | MIT |
| `github.com/resend/resend-go` | Runtime Backend (Adapter) | `internal/notifications/adapters/out/email` | Envio de emails transacionais e operacionais via API Resend | MIT |
| `github.com/getsentry/sentry-go` | Runtime Backend (Adapter) | `internal/platform/adapters/out/observability` | Monitoramento e captura de exceções em produção | Apache-2.0 |
| `github.com/posthog/posthog-go` | Runtime Backend (Adapter) | `internal/platform/adapters/out/observability` | Telemetria e métricas de produto sem dados sensíveis | MIT |
| `github.com/rivo/uniseg` v0.4.7 | Runtime Backend (Platform) | `internal/platform/text` | Segmentação e contagem de grapheme clusters (UAX #29) para o limite de 3.000 clusters e a tarifação de 1 INK por cluster (ADR-013) | MIT |
| `golangci-lint` | Dev / Quality Tooling | Pipeline de CI e `Makefile` | Análise estática e checagem de regras de código Go | GPL-3.0 (CLI externa) |
| `govulncheck` | Dev / Security Tooling | Pipeline de CI e `Makefile` | Verificação oficial de vulnerabilidades conhecidas em Go | BSD-3-Clause |
| `testcontainers-go` | Test Tooling | `tests/integration` | Subida de contêineres efêmeros de PostgreSQL para testes | MIT |
| `k6` | Test Tooling | `tests/load` | Testes de carga, estresse e validação de SLO de performance | AGPL-3.0 (CLI externa) |
| `playwright` | Test Tooling | `tools/e2e` | Testes end-to-end em navegadores reais (isolado da web; `make test-e2e` prova por gate que nada do runner é entregue) | Apache-2.0 |
| `gitleaks` | Dev / Security Tooling | Pipeline de CI e pre-commit | Varredura de credenciais e segredos no histórico Git | MIT |
| `dependabot` | Dev / Security Tooling | Repositório / GitHub | Monitoramento automatizado de novas versões e CVEs | Serviço GitHub |
| `PostgreSQL 18.x` | Infraestrutura / Dados | `infra/postgres` | Sistema relacional primário de registro e persistência ACID | PostgreSQL License |
| `Caddy 2` | Infraestrutura / Proxy | `infra/caddy` | Proxy reverso, terminação TLS automática e compressão | Apache-2.0 |
| `Cloudflare` (DNS/CDN/WAF) | Infraestrutura / Borda | `infra/cloudflare` | CDN de ativos públicos, mitigação DDoS e Turnstile | Proprietária (SaaS) |
| `Cloudflare R2` (futuro) | Infraestrutura / Storage | `internal/platform/adapters/out/storage` | Armazenamento de arquivos estáticos quando necessário | Proprietária (SaaS) |
| `Docker Compose` | Infraestrutura / Deploy | `infra/compose` | Orquestração local e de deploy do monólito na VPS | Apache-2.0 |
| `GitHub Actions` | Infraestrutura / CI | `.github/workflows` | Execução automatizada de testes e checagens no CI | Proprietária (SaaS) |

---

## 5. Política de versões e pinning

1. **Backend (Go modules):**
   - O arquivo `go.mod` deve fixar versões semânticas exatas (ou patches específicos).
   - O arquivo `go.sum` é versionado obrigatoriamente e valida os checksums criptográficos de cada dependência e suas dependências transitivas.
   - Atualizações utilizam `go get -u=patch` para patches de segurança ou atualização controlada por tarefa.
2. **Frontend (Node/TypeScript):**
   - O arquivo `package.json` define a versão exata do compilador `typescript` (sem prefixos `^` ou `~`).
   - O arquivo `package-lock.json` é mantido estritamente consistente e versionado no Git.
3. **Infraestrutura e contêineres:**
   - Imagens de contêiner em arquivos `Dockerfile` e `docker-compose.yml` utilizam tags específicas e devem apontar para digests imutáveis (`image@sha256:...`) em produção.
   - Proibido o uso de tags voláteis como `:latest`.

---

## 6. Política de licenças

O repositório adota licenças abertas permissivas para seu código e componentes runtime:
- **Licenças homologadas para runtime e bibliotecas:** MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC e PostgreSQL License.
- **Licenças restritas/proibidas no runtime:** Licenças copyleft com cláusulas fortes de reciprocidade (como AGPL, GPL, SSPL, EUPL) são expressamente proibidas no código fonte compilado da aplicação.
- **Ferramentas de desenvolvimento e teste isoladas:** Softwares e executáveis de suporte executados externamente ao binário (ex.: `golangci-lint`, `k6`) podem utilizar licenças como GPL ou AGPL, visto que não são linkados nem distribuídos junto à aplicação.

---

## 7. Gestão de vulnerabilidades (CVEs)

A segurança da cadeia de suprimentos segue as diretrizes de [SECURITY.md](SECURITY.md):
1. **Varredura obrigatória:** o comando `govulncheck ./...` é executado antes de releases e em pipelines automatizados.
2. **Monitoramento contínuo:** alertas de segurança disparados pelo Dependabot ou relatórios externos são triados conforme a matriz de impacto do Threat Model.
3. **Prazos de remediação:**
   - Vulnerabilidades com severidade **Crítica** ou **Alta** com vetor de exploração direto devem ter plano de contenção em até 24 horas e atualização em até 72 horas.
   - Vulnerabilidades **Médias** ou **Baixas** são tratadas no ciclo regular de manutenção.
4. **Isolamento como barreira:** vulnerabilidades em dependências de adapters externos não devem comprometer o núcleo de domínio caso o adapter mantenha validação estrita de fronteira.

---

## 8. Processo de atualização

1. **Atualizações de rotina:**
   - Dependências são atualizadas preferencialmente em ciclos planejados e commits isolados do tipo `build(deps): ...`.
   - É proibido atualizar dependências como efeito colateral de tarefas de regras de negócio.
2. **Critérios de aprovação de atualização:**
   - Leitura e validação prévia do changelog do fornecedor.
   - Execução bem-sucedida de todos os testes de unidade, integração, contrato e corrida (`make verify`).
   - Confirmação de ausência de novas dependências transitivas indesejadas.

---

## 9. Política de descontinuação e remoção

Toda dependência externa é adotada sob a premissa de que poderá ser removida ou substituída:
1. **Critérios de descontinuação:**
   - Biblioteca declarada como sem manutenção ou abandonada pelo mantenedor.
   - Surgimento de vulnerabilidade crítica sem previsão de correção pelo mantenedor.
   - Disponibilização de suporte nativo equivalente na biblioteca padrão do Go ou plataforma web.
   - Violação de princípios arquiteturais do projeto.
2. **Facilidade de substituição:**
   - Como toda dependência externa é confinada a um adapter que implementa uma porta da aplicação, a remoção da dependência exige apenas a reescrita ou substituição do adapter correspondente, mantendo o domínio e os casos de uso intactos.

---

## 10. Regra de admissão de novas dependências

> **Gate mandatário:** nenhuma nova biblioteca de terceiros pode ser adicionada ao projeto de forma implícita.

Qualquer proposta de adição de nova dependência externa requer:
1. **Necessidade mensurada:** problema concreto que não pode ser resolvido com código razoável utilizando a biblioteca padrão ou a plataforma web nativa.
2. **Avaliação técnica documentada:** relatório curto demonstrando por que a implementação própria não é viável ou recomendada.
3. **Análise de fornecedor:** reputação, histórico de releases, licença compatível e impacto de dependências transitivas.
4. **Isolamento arquitetural:** definição prévia de qual adapter será o único owner da dependência.
5. **Plano de contingência e remoção:** documentação de como o sistema operará caso a biblioteca precise ser removida.
6. **Aprovação formal:** criação de um Architecture Decision Record (ADR) dedicado em `docs/adr/`, aprovado formalmente antes da inclusão do pacote nos manifestos de dependência (`go.mod` ou `package.json`).
