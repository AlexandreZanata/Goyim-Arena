# Auditoria de privacidade e moderação (P20-T06)

Este documento é o registro da revisão de ciclo de vida de dados feita antes do
lançamento: o que a política (`docs/PRIVACY.md`) promete, o que o código
entrega, e a diferença entre os dois onde ela existe. A prosa é para quem lê; o
bloco JSON no fim é para a ferramenta, que o julga e recusa divergência. Reexecutar
a revisão é `make privacy-audit`.

## 1. Como a revisão foi conduzida

Duas contas sintéticas, `export-owner@arena.example.com` e
`export-other@arena.example.com`, nas superfícies que serializam dados de
titular. A pergunta não é "o dono vê os próprios dados?", e sim "alguma
superfície de um titular mostra qualquer coisa do outro?". A execução que faz a
comparação está nomeada no registro (`mixing_evidence`), e a ferramenta abre o
arquivo e exige que ele **nomeie as duas contas**: "usamos duas contas" é uma
frase; um teste que segura os dois endereços e afirma que um não aparece no
outro é evidência.

Os endereços são de domínio reservado para documentação e teste (RFC 2606). A
ferramenta recusa qualquer endereço fora dessas zonas em qualquer parte deste
documento — a validação da fase, "o relatório não contém PII real", feita de
forma mecânica.

## 2. As sete áreas

**Exportação pública da Arena.** O documento v1 (`schema_version`) carrega
identidade e estado da Arena, distribuições agregadas com supressão de baixa
contagem, contagens válidas de influência e uma página de argumentos públicos
com suas fontes. Não carrega posição individual, histórico individual de
mudança, identidade de quem atribuiu influência, email, identificador de
pagamento, IP, sinal de dispositivo, nota de moderação nem payload de pagamento
— e o portão não confia na frase: ele lê as chaves JSON que o arquivo da
superfície pode emitir e as compara, nos dois sentidos, com a allowlist
declarada aqui.

**Exportação pessoal do titular.** O documento é servido apenas ao dono,
autenticado, por link opaco de uso único com `Cache-Control: private, no-store`.
Carrega as categorias privadas do titular (conta, perfil, preferências,
histórico de nomes, sessões sem sinais de dispositivo, posições e mudanças,
rascunhos, argumentos e fontes, carteira, passes e cobrança) e declara no
próprio documento as categorias excluídas (`excluded_categories`): dados
restritos de segurança, evidência de moderação, identificadores do provedor de
pagamento e sinais antifraude.

**Exclusão de conta.** O pedido tem janela de arrependimento, a execução é
transacional, a falha da trilha de auditoria bloqueia a transição e o status
exige o próprio titular. Conteúdo público pode permanecer com autoria
anonimizada, que é o que a política publica.

**Retenção.** O cronograma é executável (`internal/profiles/domain/retention.go`)
e a tabela deste registro é comparada com ele linha a linha: classe, ação,
janela, caráter indefinido e código de razão. Retenções legais suspendem a ação
sobre os registros cobertos; as classes são independentes (uma retenção em
`sessions` não impede a anonimização de `abuse_signals`); a execução é
idempotente e falha fechada com rollback.

**Payload de analytics.** A allowlist é fechada e vive em revisão, não no ponto
de chamada: seis eventos, com as propriedades `locale` e `attributed_count`. O
portão compara o vocabulário publicado com o que o despachante aceita, nos dois
sentidos, e recusa qualquer nome de propriedade com forma de pessoa, dispositivo
ou segredo. Não há forma livre de texto: a única string admitida é um locale
validado contra o catálogo.

**Logs.** A redação central (`internal/platform/logging`) substitui valores
sensíveis por `[REDACTED]` e a suíte prova que nenhum valor sensível atravessa.
O que os logs carregam é decisão de código, e é por isso que a evidência é a
suíte e não a prosa.

**Moderação: recursos e reversões.** A fila de moderação nunca expõe quem
denunciou, a evidência restrita ou a justificativa interna: o moderador recebe
alvo, tipo, prioridade e estado. Recursos e reversões ficam na trilha
append-only, que é classe `referential_logs` — retida como evidência, sem
conteúdo de titular além da referência.

## 3. Baixa contagem

O limiar é `LowCountThreshold = 5`. Uma população abaixo dele não publica total,
distribuição nem contagem de mudanças: o agregado publica `suppressed = true` e
zeros. A ferramenta não confia no número publicado — ela confere o número e
exercita o comportamento nas duas bordas (limiar − 1 não publica, o limiar
publica, zero não publica).

## 4. Achados

Dois, nenhum Crítico nem Alto.

**PRV-01 (Baixa, aceito, `docs/privacy`)** — `docs/PRIVACY.md` §6 diz que a
exportação pública da Arena carrega "versão de esquema e aviso metodológico". A
versão existe (`schema_version`); o aviso metodológico não. `methodology_version`
existe, mas no endpoint de estatísticas, que é outro documento. O residual é
aceito: o aviso é redação e revisão, não mecanismo, e vai junto da publicação do
beta.

**PRV-02 (Média, aceito, `identity/obs`)** — a política descreve a classe
restrita `abuse_signals` como "IP de origem e user agent registrados na sessão",
anonimizados após sete dias. O que a sessão registra é `r.RemoteAddr`, o par
endereço:porta do **par direto** — em produção, o proxy, não a origem. O
`clientip.Resolver` que resolve o endereço de origem existe e é composto para
rate limit; a identidade não o usa. O efeito é uma referência de prevenção que
não contém o que a política diz que contém (o que é favorável ao titular e inútil
para a prevenção), e o residual é aceito com trabalho seguinte nomeado.

## 5. Limites desta revisão

O portão é mecânico onde o mecanismo existe: chaves de exportação, tabela de
retenção, limiar, vocabulário de eventos, caminhos citados. O que ele **não**
decide é se a categoria certa está certa — se um campo novo, com nome inocente,
carrega dado que não deveria. Isso continua sendo a revisão humana das
categorias, registrada aqui como limite declarado, e não como cobertura.

A revisão de moderação é de superfície: o gate lê o que serializa e recusa
evidência restrita por nome. A neutralidade do processo decisório
(`internal/moderation/application/neutrality_test.go`) é uma propriedade de
produto, verificada pela suíte, não por este documento.

```json
{
  "version": 1,
  "audited_on": "2026-09-22",
  "accounts": [
    {
      "id": "owner",
      "email": "export-owner@arena.example.com",
      "note": "Titular autenticado: recebe a própria exportação e exerce os próprios direitos."
    },
    {
      "id": "outsider",
      "email": "export-other@arena.example.com",
      "note": "Segundo titular, nunca autenticado nas superfícies do primeiro: existe para provar que nada dele aparece."
    }
  ],
  "mixing_evidence": {
    "path": "internal/profiles/adapters/http/export_test.go",
    "note": "Segura as duas contas, serve a exportação de uma e afirma que a marca da outra não aparece no documento, e que o token forjado ou de dono estrangeiro responde não encontrado."
  },
  "allowlists": [
    {
      "id": "public-export",
      "surface": "public",
      "path": "internal/transparency/adapters/http/export.go",
      "note": "Documento v1 da exportação pública da Arena. Agregados, instantes, versão de esquema e conteúdo já público; nenhum campo sobre uma pessoa.",
      "keys": [
        "agree",
        "arena",
        "arguments",
        "category",
        "closes_at",
        "content",
        "context",
        "created_at",
        "current",
        "description",
        "disagree",
        "distinct_people",
        "id",
        "influence",
        "influenced_authors",
        "initial",
        "items",
        "language",
        "next_cursor",
        "parent_id",
        "participants_total",
        "position_changes",
        "positions",
        "published_at",
        "relation",
        "schema_version",
        "slug",
        "sources",
        "statement",
        "status",
        "suppressed",
        "undecided",
        "url",
        "valid_attributions",
        "withdrawn_at"
      ]
    },
    {
      "id": "personal-export",
      "surface": "subject",
      "path": "internal/profiles/application/export_document.go",
      "note": "Documento da exportação pessoal, servido só ao dono. Pode carregar dado privado do titular; não pode carregar identificador do provedor de pagamento, payload de webhook, evidência de moderação, sinal de dispositivo, endereço de rede nem segredo.",
      "keys": [
        "account",
        "amount",
        "amount_minor",
        "arena_drafts",
        "arena_id",
        "arena_slug",
        "arena_statement",
        "arguments",
        "balance_free",
        "balance_purchased",
        "billing",
        "bucket",
        "cancel_at_period_end",
        "category",
        "changed_at",
        "checkout_intents",
        "consumed_at",
        "consumptions",
        "content",
        "context",
        "created_at",
        "currency",
        "current_period_end",
        "current_period_start",
        "current_position",
        "description",
        "email",
        "email_verified",
        "excluded_categories",
        "expires_at",
        "from_position",
        "generated_at",
        "id",
        "initial_position",
        "interface_locale",
        "language",
        "lots",
        "market",
        "marketing_opt_in",
        "operation",
        "origin",
        "paid_at",
        "parent_id",
        "passes",
        "position_changes",
        "positions",
        "preferences",
        "product_id",
        "profile",
        "quantity",
        "relation",
        "remaining",
        "revoked_at",
        "schema_version",
        "sessions",
        "sources",
        "statement",
        "status",
        "subscriptions",
        "timezone",
        "to_position",
        "transactions",
        "updated_at",
        "url",
        "username",
        "username_history",
        "version",
        "wallet",
        "withdrawn_at"
      ]
    }
  ],
  "retention": [
    { "class": "tokens", "action": "purge", "window_hours": 720, "indefinite": false, "reason_code": "single_use_credential" },
    { "class": "sessions", "action": "purge", "window_hours": 720, "indefinite": false, "reason_code": "terminal_session" },
    { "class": "referential_logs", "action": "retain", "window_hours": 0, "indefinite": true, "reason_code": "administrative_evidence" },
    { "class": "exports", "action": "purge", "window_hours": 24, "indefinite": false, "reason_code": "expired_export_document" },
    { "class": "abuse_signals", "action": "anonymize", "window_hours": 168, "indefinite": false, "reason_code": "prevention_reference" },
    { "class": "billing", "action": "retain", "window_hours": 0, "indefinite": true, "reason_code": "financial_evidence" }
  ],
  "low_count": {
    "threshold": 5,
    "note": "Uma população abaixo do limiar publica suppressed=true e zeros: nem total, nem distribuição, nem contagem de mudanças."
  },
  "analytics": [
    { "event": "account.registration_submitted", "properties": ["locale"] },
    { "event": "account.signed_in", "properties": ["locale"] },
    { "event": "arena.argument_published", "properties": ["locale"] },
    { "event": "arena.influence_assigned", "properties": ["attributed_count", "locale"] },
    { "event": "arena.position_changed", "properties": ["locale"] },
    { "event": "arena.position_confirmed", "properties": ["locale"] }
  ],
  "areas": [
    {
      "key": "public-export",
      "verdict": "gap",
      "finding": "PRV-01",
      "evidence": [
        "internal/transparency/adapters/http/export.go",
        "internal/transparency/application/export.go",
        "internal/transparency/domain/export.go",
        "docs/TRANSPARENCY.md"
      ],
      "execution": "go test ./internal/transparency/...",
      "note": "A superfície não carrega nada sobre uma pessoa e a allowlist fecha as chaves emitidas; o aviso metodológico que a política §6 promete não está no documento, e é o achado PRV-01."
    },
    {
      "key": "personal-export",
      "verdict": "mitigated",
      "finding": "",
      "evidence": [
        "internal/profiles/application/export_document.go",
        "internal/profiles/adapters/http/export_test.go"
      ],
      "execution": "go test ./internal/profiles/adapters/http/...",
      "note": "O documento do dono não carrega nenhuma marca do segundo titular, nem identificador de provedor, webhook, evidência de moderação, sinal de dispositivo ou segredo; link de uso único com no-store."
    },
    {
      "key": "deletion",
      "verdict": "mitigated",
      "finding": "",
      "evidence": [
        "internal/profiles/application/deletion.go",
        "internal/profiles/application/deletion_test.go",
        "docs/PRIVACY.md"
      ],
      "execution": "go test ./internal/profiles/application/... -run Deletion",
      "note": "Janela de arrependimento, execução transacional, status só do titular e trilha de auditoria que bloqueia a transição quando falha."
    },
    {
      "key": "retention",
      "verdict": "gap",
      "finding": "PRV-02",
      "evidence": [
        "internal/profiles/domain/retention.go",
        "internal/profiles/application/retention.go",
        "internal/platform/clientip/clientip.go"
      ],
      "execution": "go test ./internal/profiles/application/... -run Retention",
      "note": "A tabela publicada é idêntica ao cronograma que o job aplica, classe por classe, e as retenções legais suspendem a ação; a classe abuse_signals guarda o par direto em vez do IP de origem resolvido, e é o achado PRV-02."
    },
    {
      "key": "analytics-payload",
      "verdict": "mitigated",
      "finding": "",
      "evidence": [
        "internal/platform/observability/events.go",
        "internal/platform/observability/scrub.go"
      ],
      "execution": "go test ./internal/platform/observability/...",
      "note": "Seis eventos, propriedades mínimas com domínio de valor fechado (locale do catálogo e contagem limitada), sem forma livre de texto por onde um email, um rascunho ou um payload de pagamento pudessem viajar."
    },
    {
      "key": "logs",
      "verdict": "mitigated",
      "finding": "",
      "evidence": [
        "internal/platform/logging/logging.go",
        "internal/platform/logging/logging_test.go"
      ],
      "execution": "go test ./internal/platform/logging/...",
      "note": "A redação central substitui valor sensível por [REDACTED] e a suíte prova que nenhum valor sensível atravessa o logger."
    },
    {
      "key": "moderation",
      "verdict": "mitigated",
      "finding": "",
      "evidence": [
        "internal/moderation/adapters/http/handler.go",
        "internal/moderation/application/appeals.go",
        "internal/moderation/application/neutrality_test.go"
      ],
      "execution": "go test ./internal/moderation/...",
      "note": "A fila expõe alvo, tipo, prioridade e estado — nunca quem denunciou, a evidência ou a justificativa interna; recursos e reversões ficam na trilha append-only."
    }
  ],
  "findings": [
    {
      "id": "PRV-01",
      "title": "A exportação pública da Arena não carrega o aviso metodológico que docs/PRIVACY.md §6 promete",
      "severity": "Baixa",
      "status": "accepted",
      "owner": "docs/privacy",
      "accepted_on": "2026-09-22",
      "area": "public-export",
      "evidence": [
        "docs/PRIVACY.md",
        "internal/transparency/adapters/http/export.go"
      ],
      "plan": "Publicar o aviso metodológico na exportação pública v1 quando o método estatístico for publicado, junto da revisão jurídica de privacidade do beta; até lá o residual é a promessa sem o campo."
    },
    {
      "id": "PRV-02",
      "title": "A referência restrita de prevenção guarda o par direto (r.RemoteAddr) em vez do IP de origem resolvido",
      "severity": "Média",
      "status": "accepted",
      "owner": "identity/obs",
      "accepted_on": "2026-09-22",
      "area": "retention",
      "evidence": [
        "internal/identity/adapters/html/handler.go",
        "internal/identity/adapters/http/sessions.go",
        "internal/platform/clientip/clientip.go"
      ],
      "plan": "Resolver o endereço pelo clientip.Resolver já composto no bootstrap nos três pontos que registram IPAddress, mantendo a classe abuse_signals e a janela de sete dias inalteradas; a evidência de prevenção passa a ser o que a política publica."
    }
  ]
}
```
