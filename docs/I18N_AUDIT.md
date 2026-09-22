# Auditoria final de internacionalização

Registro da auditoria de internacionalização da release (P20-T09). Ele tem duas
metades, de propósito: a prosa abaixo, para quem lê, e um bloco de máquina no
fim, para o portão — `make i18n-audit`, dentro de `make verify`.

As duas metades dizem coisas diferentes. A prosa explica o que foi medido e o
que ficou em aberto; o bloco carrega o que a auditoria **afirma** sobre a
árvore, e `tools/i18nrelease` recalcula cada afirmação a partir da árvore a cada
execução. Um registro que só narrasse o que foi feito seria indistinguível de um
que narra o que deveria ter sido feito; a comparação é o que separa os dois.

**Quem executa o quê.** `make i18n-audit` roda as áreas que não precisam de
navegador, com os mesmos comandos que o bloco registra, e recusa qualquer uma
que responda diferente de zero — depois julga o registro contra a árvore. As
jornadas de navegador são a única área que esse portão não executa: elas exigem
um Chromium e um PostgreSQL descartável, que `tools/e2e/harness.sh` compõe e o
CI roda como job próprio (`make test-e2e`). O registro guarda essa execução com
a data, e a auditoria prova que as jornadas realmente carregam a dimensão de
idioma medindo o módulo de onde a suíte a lê.

## Áreas executadas

As doze áreas que a fase nomeia, o comando que executa cada uma e a evidência em
que ela se apoia. Os comandos são os portões entregues, não cópias privadas: uma
auditoria que executasse as suas próprias suítes provaria que a auditoria passa,
e continuaria passando depois que os portões que embarcam deixassem de existir.
A execução de cada uma está no bloco do fim deste documento.

| Área | Comando | Evidência |
| --- | --- | --- |
| `catalog-coverage` | `make generate-check` | `locales/pt-BR/arenas.json` |
| `pseudo-locale` | `go test -tags pseudolocale ./internal/i18n/...` | `internal/i18n/pseudo_enabled.go` |
| `catalog-snapshots` | `go test ./internal/notifications/adapters/renderer/...` | `internal/notifications/adapters/renderer/snapshot_test.go` |
| `emails` | `go test ./internal/notifications/...` | `locales/pt-BR/email.json` |
| `problem-details` | `go test ./internal/platform/httperror/...` | `locales/pt-BR/errors.json` |
| `seo` | `go test ./internal/arenas/adapters/html/...` | `internal/arenas/adapters/html/templates.go` |
| `money` | `make test-web` | `web/src/i18n/formats.ts` |
| `plural` | `make test-web` | `web/src/i18n/translator.ts` |
| `timezone` | `make test-web` | `web/tests/i18n/formats.test.ts` |
| `cache` | `go test ./internal/arenas/adapters/html/...` | `internal/platform/httpcache/policy.go` |
| `hardcoded-text` | `make audit-i18n` | `tools/i18naudit/audit.go` |
| `browser-journeys` | `make test-e2e` | `tools/e2e/support/locales.js` |

## O que foi medido

**Cobertura do catálogo.** Os dois locales têm 200 mensagens cada, nos mesmos
cinco namespaces, com os mesmos placeholders: zero chave ausente, zero chave
extra, zero divergência de placeholder, zero mensagem vazia. As 200 chaves são
**alcançáveis a partir do código entregue** — 165 escritas por extenso no ponto
de chamada e 6 hastes terminadas em ponto que o completam em tempo de execução,
como `arenas.document.status.` seguida do status. Zero órfã: 100% das chaves
ativas cobertas, medido, não contado pelo gerador que as produz.

**Snapshots e emails.** Três templates transacionais (`verification`,
`password_reset`, `password_changed`) em dois locales: as seis imagens
versionadas existem e são comparadas byte a byte pelo teste do renderer, que
nunca reescreve o artefato.

**Problem Details.** Sete códigos estáveis (`validation`, `unauthorized`,
`forbidden`, `not_found`, `conflict`, `rate_limited`, `internal`) declarados nos
dois locales: nenhum código existe em um idioma e falta no outro.

**SEO.** Zero `hreflang` na árvore entregue, que é o estado correto enquanto não
houver página realmente traduzida e equivalente — §7 permite o alternate
somente para páginas que existem nos dois idiomas. O documento da Arena declara
o `content_language` que a Arena tem, e o teste do adapter prova que ele não
muda com o idioma de quem lê. O que a auditoria achou é que **esse documento
não é servido**: a rota `/d/{slug}` existe no registro de rotas, o adapter, os
templates e os testes existem, e nenhuma composição registra o handler — é o
I18N-05.

**Cache.** Oito pontos marcam resposta como publicamente cacheável, e **um**
deles varia pelo header `Accept-Language` bruto: é o item I18N-02. A auditoria
não decidiu o remédio — servir o documento localizado em cache privado e
resolver o documento público pelo parâmetro explícito são ambos conformes, e a
escolha é de produto. O que ela não aceita é o silêncio.

**Texto hardcoded, língua e direção.** `make audit-i18n` varre a árvore entregue
e não encontra nenhuma frase própria em nó de texto, nenhum `<html>` com `lang`
literal ou sem `dir`, e nenhuma propriedade física de CSS — as três coisas que
fazem uma página mentir sobre o idioma que ela está falando.

**Jornadas de navegador.** A suíte agora é dirigida em **cada** idioma que o
produto embarca, com contexto próprio por jornada, e cada página alcançada é
cobrada pelo idioma que aquele contexto pediu (`navigator.language` lido da
própria página, nunca uma constante da jornada) — mais uma guarda de vacuidade
que recusa um contexto cujo idioma não seja um dos da suíte. O documento de
Arena, esse, é cobrado pelo `content_language` semeado pelo harness, nos dois
idiomas: é assim que "trocar o idioma da interface não traduz o conteúdo" vira
fato medido.

## Findings

| ID | Severidade | Estado | Área | Dono | Trabalho seguinte |
| --- | --- | --- | --- | --- | --- |
| I18N-01 | medium | fixed | `browser-journeys` | `i18n` | — |
| I18N-02 | medium | open | `cache` | `platform/transparency` | decidir entre cache privado do documento localizado e resolução do documento público só pelo parâmetro explícito/cookie, e implementar na microtarefa seguinte |
| I18N-03 | low | fixed | `browser-journeys` | `i18n` | — |
| I18N-04 | low | accepted | `catalog-coverage` | `i18n` | revisar quando o catálogo mudar de forma |
| I18N-05 | medium | open | `seo` | `bootstrap/arenas` | compor o documento público da Arena no processo entregue (ou decidir que ele não existe) e resolver o `canonical` que aponta para ele |

**I18N-01 (medium, corrigido nesta tarefa).** A jornada de participação
selecionava a opção de atribuição pelo **rótulo traduzido** do campo
(`getByLabel(/argumento do parceiro/)`), então ela só passava no idioma em que
foi escrita — e "as jornadas passam" era uma afirmação sobre um locale. O
seletor passou a ser o nome do campo, que é contrato do formulário e não texto
de catálogo.

**I18N-02 (medium, aberto).** `internal/transparency/adapters/http/handler.go`
serve o documento HTML de transparência com `Cache-Control: public` e
`Vary: Accept-Language`. O conteúdo está certo — a locale resolvida passa pela
allowlist e valor desconhecido não se reflete —, mas §4 exige que cache público
varie por uma **chave de locale controlada** e nunca pelo header bruto. Dono,
trabalho seguinte e a data da decisão estão na tabela; nenhuma das duas
correções é escolhida aqui.

**I18N-03 (low, corrigido nesta tarefa).** Nenhuma jornada verificava o idioma
em que foi servida: rodar a suíte duas vezes com contextos de idiomas diferentes
seria a mesma execução repetida. `tools/e2e/support/locales.js` acrescentou a
afirmação em cada página alcançada — e a guarda que a impede de ser vacuidade.

**I18N-05 (medium, aberto).** A superfície mais internacionalizada do produto — o
documento público da Arena, com `lang`, `og:locale`, JSON-LD `inLanguage`, o
status localizado e o cache público por 60s — **não é servida pelo processo que
embarca**. `internal/arenas/adapters/html/routes.go` declara `GET /d/{slug}` e
nada em `internal/bootstrap` constrói `arenashtml.NewHandler`: quem pede
`/d/<slug>` recebe 404, que foi exatamente o que a jornada de navegador recebeu
antes de esta auditoria deixar de afirmar sobre uma página que ninguém serve. A
consequência não é só a página ausente: o `canonical` que a **página de
participação entregue** anuncia é `/d/<slug>`
(`internal/arenas/adapters/html/participation.go:850`), ou seja, o endereço
canônico que o produto publica responde 404. É a mesma família do achado do
drill de desastre (a API JSON do contrato não composta), com dono, trabalho
seguinte e a validação da fase que ela fura: enquanto isso, as afirmações de §7
sobre essa página são verdadeiras para os testes e os templates, não para o
binário.

**I18N-04 (low, aceito).** Cinco literais com forma de chave que o catálogo não
declara: `auth.password_reset_request` e `auth.password_reset_confirm` em
`internal/platform/ratelimit/policy.go` (são identificadores de ação do rate
limit, que compartilham o prefixo do namespace) e três fixtures de teste que
nomeiam chave inexistente de propósito — `arenas.document.ghost` e
`errors.ghost.title` em `internal/i18n/i18n_test.go`, e `email.verification_1`
em `internal/notifications/domain/types_test.go`. Nenhum deles é mensagem
faltando; a auditoria os reporta para que um literal novo tenha que ser
explicado por quem o escrever.

## Revisão humana do en-US

**Pendente.** A revisão humana do catálogo traduzido é obrigatória antes de
release (§10), e esta auditoria não a substitui: ela prova paridade de chaves,
igualdade de placeholders, cobertura total e ligação ao código — propriedades
mecânicas. Não prova que o inglês está certo, e um catálogo que compila não é um
catálogo revisado. Quem decide é o titular do repositório, e o que falta está
declarado no bloco: a leitura integral do catálogo `en-US` por um revisor humano
antes da publicação do beta.

## Limitações

1. A auditoria mede paridade, placeholders, cobertura e ligação ao código; a
   **adequação editorial da tradução** depende da revisão humana, que está
   pendente e registrada acima.
2. As jornadas de navegador foram executadas no commit registrado abaixo. O CI
   as reexecuta em `make test-e2e`; este portão não as roda, e por isso o que ele
   prova sobre elas é estrutural (as suítes existem, iteram pelos dois idiomas e
   estão ligadas a um alvo do `Makefile`).
3. A varredura do código entregue lê Go, TypeScript, JavaScript e HTML em
   `internal/`, `cmd/`, `tools/`, `web/src/` e `tests/`, e exclui o diretório da
   própria auditoria, os catálogos gerados e os arquivos de teste na varredura
   de cache e de formatação. O diretório da auditoria é excluído porque contém,
   por necessidade, as palavras que ela procura — e não é superfície que responde
   a cliente.
4. Zero variantes plurais no catálogo hoje. O que está provado é o **mecanismo**
   (`Intl.PluralRules` na web, chaves terminadas em categoria CLDR no tradutor e
   nos testes), não que alguma mensagem precise delas.
5. Moeda e tempo são provados pelos testes dos formatadores entregues
   (`Intl.NumberFormat` com moeda explícita e minor units; instantes com fuso
   nomeado, inclusive numa transição de horário de verão). A auditoria não
   rederiva a base CLDR do runtime.
6. O total de chaves alcançáveis conta uma chave composta por haste terminada em
   ponto como alcançável; uma chave que só existisse por concatenação sem haste
   reconhecível apareceria como órfã e precisaria de decisão, não de supressão.
7. A auditoria não decide remédio para violação de regra do padrão: ela nomeia,
   com dono e trabalho seguinte, como em I18N-02. Quem reduz o padrão não é ela.

<!-- i18n-release:facts -->
```json
{
  "version": "i18n-release/1",
  "date": "2026-09-22",
  "areas": [
    {
      "id": "catalog-coverage",
      "command": "make generate-check",
      "evidence": "locales/pt-BR/arenas.json",
      "status": "pass",
      "seconds": 0.204447348
    },
    {
      "id": "pseudo-locale",
      "command": "go test -tags pseudolocale ./internal/i18n/...",
      "evidence": "internal/i18n/pseudo_enabled.go",
      "status": "pass",
      "seconds": 0.062185111
    },
    {
      "id": "catalog-snapshots",
      "command": "go test ./internal/notifications/adapters/renderer/...",
      "evidence": "internal/notifications/adapters/renderer/snapshot_test.go",
      "status": "pass",
      "seconds": 0.209632395
    },
    {
      "id": "emails",
      "command": "go test ./internal/notifications/...",
      "evidence": "locales/pt-BR/email.json",
      "status": "pass",
      "seconds": 0.22048345
    },
    {
      "id": "problem-details",
      "command": "go test ./internal/platform/httperror/...",
      "evidence": "locales/pt-BR/errors.json",
      "status": "pass",
      "seconds": 0.067348372
    },
    {
      "id": "seo",
      "command": "go test ./internal/arenas/adapters/html/...",
      "evidence": "internal/arenas/adapters/html/templates.go",
      "status": "pass",
      "seconds": 0.08081115
    },
    {
      "id": "money",
      "command": "make test-web",
      "evidence": "web/src/i18n/formats.ts",
      "status": "pass",
      "seconds": 1.085125913
    },
    {
      "id": "plural",
      "command": "make test-web",
      "evidence": "web/src/i18n/translator.ts",
      "status": "pass",
      "seconds": 0.77063121
    },
    {
      "id": "timezone",
      "command": "make test-web",
      "evidence": "web/tests/i18n/formats.test.ts",
      "status": "pass",
      "seconds": 0.784738019
    },
    {
      "id": "cache",
      "command": "go test ./internal/arenas/adapters/html/...",
      "evidence": "internal/platform/httpcache/policy.go",
      "status": "pass",
      "seconds": 0.08013448
    },
    {
      "id": "hardcoded-text",
      "command": "make audit-i18n",
      "evidence": "tools/i18naudit/audit.go",
      "status": "pass",
      "seconds": 0.123141233
    },
    {
      "id": "browser-journeys",
      "command": "make test-e2e",
      "evidence": "tools/e2e/support/locales.js",
      "status": "pass",
      "seconds": 10.763638758
    }
  ],
  "claims": {
    "locales": [
      "en-US",
      "pt-BR"
    ],
    "namespaces": [
      "arenas",
      "auth",
      "email",
      "errors",
      "transparency"
    ],
    "keys_per_locale": {
      "en-US": 200,
      "pt-BR": 200
    },
    "placeholder_divergence": 0,
    "empty_messages": 0,
    "orphan_keys": 0,
    "unknown_references": 5,
    "email_goldens": 6,
    "error_code_drift": 0,
    "plural_variants": 0,
    "hreflang": 0,
    "to_locale_string": 0,
    "vary_by_raw_accept_language": 1,
    "public_cacheable_surfaces": 8,
    "journey_locales": [
      "en-US",
      "pt-BR"
    ]
  },
  "findings": [
    {
      "id": "I18N-01",
      "severity": "medium",
      "status": "fixed",
      "area": "browser-journeys",
      "owner": "i18n",
      "next": "",
      "evidence": "tools/e2e/specs/participation.spec.js (the attribution option was selected by its translated label)",
      "detail": "a jornada de participação selecionava a opção de atribuição pelo rótulo traduzido e só passava no idioma em que foi escrita"
    },
    {
      "id": "I18N-02",
      "severity": "medium",
      "status": "open",
      "area": "cache",
      "owner": "platform/transparency",
      "next": "decidir entre cache privado do documento localizado e resolução do documento público só pelo parâmetro explícito/cookie, e implementar na microtarefa seguinte",
      "evidence": "internal/transparency/adapters/http/handler.go:257 (w.Header().Set(\"Vary\", \"Accept-Language\"))",
      "detail": "resposta publicamente cacheável varia pelo header Accept-Language bruto, que §4 proíbe"
    },
    {
      "id": "I18N-03",
      "severity": "low",
      "status": "fixed",
      "area": "browser-journeys",
      "owner": "i18n",
      "next": "",
      "evidence": "tools/e2e/support/locales.js (expectInterfaceLanguage and expectContentLanguage)",
      "detail": "nenhuma jornada verificava o idioma em que foi servida, então rodar em dois idiomas seria a mesma execução duas vezes"
    },
    {
      "id": "I18N-05",
      "severity": "medium",
      "status": "open",
      "area": "seo",
      "owner": "bootstrap/arenas",
      "next": "compor o documento público da Arena no processo entregue (ou decidir que ele não existe) e resolver o canonical que aponta para ele",
      "evidence": "internal/arenas/adapters/html/routes.go:20 (GET /d/{slug} declarada) e internal/bootstrap/participation.go (só as rotas de participação registradas); internal/arenas/adapters/html/participation.go:850 (o canonical entregue aponta para /d/\u003cslug\u003e)",
      "detail": "o documento público da Arena, com lang, og:locale e JSON-LD inLanguage, não é composto no processo entregue: a rota responde 404 e o canonical da página de participação aponta para ela"
    },
    {
      "id": "I18N-04",
      "severity": "low",
      "status": "accepted",
      "area": "catalog-coverage",
      "owner": "i18n",
      "next": "revisar quando o catálogo mudar de forma: um literal novo com forma de chave tem que ser explicado por quem o escrever",
      "evidence": "auth.password_reset_request e auth.password_reset_confirm em internal/platform/ratelimit/policy.go; arenas.document.ghost e errors.ghost.title em internal/i18n/i18n_test.go; email.verification_1 em internal/notifications/domain/types_test.go",
      "detail": "literais com forma de chave que não são chave: identificadores de ação do rate limit e fixtures que nomeiam chave inexistente de propósito"
    }
  ],
  "review": {
    "locale": "en-US",
    "state": "pending",
    "reviewer": "",
    "date": "",
    "scope": [],
    "decider": "titular do repositório",
    "missing": [
      "leitura integral do catálogo en-US por revisor humano antes da publicação: a auditoria prova paridade, placeholders e ligação ao código, não a adequação editorial da tradução"
    ]
  },
  "limits": [
    "A auditoria mede paridade, placeholders, cobertura e ligação ao código; a adequação editorial da tradução depende da revisão humana, que está pendente e registrada neste documento.",
    "As jornadas de navegador foram executadas no commit registrado; o CI as reexecuta em make test-e2e e este portão não as roda, então o que ele prova sobre elas é estrutural.",
    "A varredura lê Go, TypeScript, JavaScript e HTML em internal/, cmd/, tools/, web/src/ e tests/, e exclui o diretório da própria auditoria, os catálogos gerados e os arquivos de teste na varredura de cache e formatação.",
    "Zero variantes plurais hoje: o que está provado é o mecanismo (Intl.PluralRules na web e as chaves de categoria que o tradutor lê), não que alguma mensagem precise delas.",
    "Moeda e tempo são provados pelos testes dos formatadores entregues; a auditoria não rederiva a base CLDR do runtime.",
    "Uma chave composta por haste terminada em ponto conta como alcançável; uma chave que só existisse por concatenação sem haste reconhecível apareceria como órfã e precisaria de decisão, não de supressão.",
    "A auditoria não escolhe remédio para violação do padrão: ela nomeia, com dono e trabalho seguinte, como em I18N-02.",
    "As jornadas de navegador afirmam apenas sobre superfícies que o processo entregue serve. A prova em navegador do documento público da Arena não existe porque a rota não está composta (I18N-05); o que a auditoria mede ali é o teste do adapter, que roda sobre o handler e não sobre o binário."
  ]
}
```
<!-- /i18n-release:facts -->
