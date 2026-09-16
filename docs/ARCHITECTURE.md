# Arquitetura

**Status:** arquitetura inicial aprovada

**Estilo:** API-first, monólito modular, ports and adapters

## 1. Objetivos

- frontend substituível e sem dependências de runtime de terceiros;
- backend reutilizável por web, worker, CLI e futuros clientes;
- regras de negócio independentes de transporte e fornecedores;
- operação eficiente em uma VPS de 16 GB;
- conteúdo público rápido, acessível e indexável;
- transações fortes para wallet, posições e atribuições;
- evolução horizontal baseada em medidas, sem reescrita precoce.

## 2. Topologia inicial

```text
Browser
HTML semântico · CSS nativo · TypeScript→ESM · Web Components
   │
   ▼
Cloudflare
DNS · CDN · WAF · Turnstile · cache público
   │
   ▼
Caddy
TLS · headers · reverse proxy · compressão
   │
   ▼
HTTP adapter ──────────────── Worker adapter / CLI adapter
   │                                  │
   └──────────── Application use cases┘
                      │
                    Domain
                      │
                 Outbound ports
             ┌────────┼─────────┐
             ▼        ▼         ▼
         PostgreSQL  Stripe   Email/Telemetry

PostgreSQL 18 é a única fonte persistente inicial.
```

Server e worker usam o mesmo módulo Go e casos de uso. A aplicação começa como uma unidade de deploy, sem acoplar o domínio à topologia.

## 3. Regra de dependência

Dependências apontam para dentro:

```text
adapters ──► application ──► domain
bootstrap ─► adapters + application
domain ────► biblioteca padrão apenas
```

- domínio não conhece SQL, HTTP, JSON, cookies ou fornecedores;
- aplicação conhece o domínio e declara ports pequenos;
- adapters implementam ports e fazem tradução;
- bootstrap instancia implementações e configuração;
- módulos de domínio não importam detalhes internos uns dos outros;
- ciclos são proibidos e verificados pelo grafo de packages.

“Desacoplado” não significa abstração para tudo. Só existe interface onde há fronteira, efeito externo ou mais de uma execução relevante. Abstração sem caso de uso é removida.

## 4. Módulos de negócio

- `identity`: contas, credenciais, sessões e recuperação.
- `profiles`: usernames, locale e perfil público.
- `arenas`: rascunho, publicação, fechamento, categoria e idioma.
- `positions`: posição inicial, atual e mudanças.
- `arguments`: argumentos, respostas, fontes e retirada.
- `persuasion`: elegibilidade, atribuições e métricas públicas.
- `wallet`: ledger, buckets e consumo atômico.
- `billing`: catálogo, checkout, assinatura e webhooks.
- `moderation`: denúncias, decisões, recursos e sanções.
- `transparency`: agregados públicos e exportações versionadas.
- `audit`: eventos administrativos e integridade.
- `notifications`: mensagens transacionais.
- `jobs`: execução persistente, lease e retry.

Cada módulo expõe apenas comandos, queries e eventos públicos próprios. Acesso direto às tabelas de outro módulo é proibido fora de projeções explicitamente aprovadas.

## 5. Fluxo de um caso de uso

```text
HTTP request
  → parse/validate transport
  → command tipado
  → authorization context
  → application use case
  → domain invariants
  → transaction boundary
  → outbound ports
  → result tipado
  → HTTP representation
```

Erros de domínio são estáveis e independentes de status HTTP. O adapter mapeia erro para Problem Details. Regras não são repetidas em handlers, jobs ou SQL.

## 6. Contratos

- OpenAPI 3.1 é o contrato HTTP versionado.
- APIs públicas vivem sob `/api/v1`; mudanças incompatíveis criam nova versão.
- DTOs de transporte não são entidades de domínio.
- tipos TypeScript são gerados do contrato por tooling interno do repositório; código gerado não recebe lógica manual.
- contract tests garantem que implementação e documento permanecem iguais.
- idempotency keys fazem parte dos contratos de mutações repetíveis.
- paginação pública usa cursor opaco, nunca offset profundo.

O frontend é um consumidor da API, não uma exceção com acesso privilegiado ao banco.

## 7. Organização do repositório

```text
cmd/
  arena/                    server, worker e comandos operacionais
internal/
  <module>/
    domain/                 entidades, values, policies e erros
    application/            commands, queries, use cases e ports
    adapters/
      in/http/
      in/jobs/
      out/postgres/
  platform/                 adapters compartilhados estritamente técnicos
  bootstrap/                composition root
api/
  openapi.json              contrato HTTP fonte de verdade
tools/
  contractgen/              gerador Go interno para tipos TypeScript
web/
  src/
    core/                    http, lifecycle, events, i18n e tipos básicos
    contracts/              tipos gerados do contrato
    components/
      primitives/           componentes sem domínio
      arenas/
      arguments/
      wallet/
    pages/                   composition roots de cada página
    styles/
      reset.css
      tokens.css
      base.css
      layout.css
  public/                    assets estáticos
  generated/                 JS/CSS versionados para deploy, não editados
db/
  migrations/
  queries/
infra/
  compose/
  caddy/
  postgres/
docs/
  adr/
tests/
  contract/
  integration/
  e2e/
  load/
```

## 8. Arquitetura dos componentes web

Componentes são pequenos elementos nativos, não miniaplicações. Um componente:

- recebe dados serializáveis e dependências por contrato;
- mantém apenas estado visual local;
- usa `AbortController` para cancelar efeitos ao sair do DOM;
- emite eventos sem conhecer o consumidor;
- não chama endpoints arbitrários: usa um client tipado injetado;
- não conhece autenticação, analytics ou cache diretamente;
- funciona com teclado, leitores de tela e motion reduzido;
- possui CSS local e teste de contrato visual/comportamental.

Page controllers compõem componentes e casos de navegação. Não existe singleton global mutável nem event bus genérico.

## 9. HTML, SEO e progressive enhancement

Go entrega documento HTML semântico com `html/template`, metadados e conteúdo público necessário à indexação. TypeScript registra componentes e aprimora filtros, formulários, paginação e atualização parcial usando `fetch`.

Formulários críticos têm endpoint HTTP normal e continuam funcionais sem JavaScript sempre que possível. Isso melhora acessibilidade, resiliência e testes. Não há duplicação de regra: tanto resposta HTML quanto JSON chamam o mesmo caso de uso.

## 10. Separação entre público e privado

- `/d/:slug`: documento público cacheável.
- `/api/v1/public/*`: dados públicos com TTL e ETag.
- `/api/v1/me/*`, wallet, auth, checkout e admin: `private, no-store`.
- respostas com `Set-Cookie` nunca entram em cache compartilhado.
- HTML público não inclui email, saldo, posição individual ou autoria de atribuição.

O bloqueio do agregado antes da escolha é regra de experiência, não segredo. Metadados e previews não mostram o agregado.

## 11. Persistência e consistência

PostgreSQL é a fonte de verdade. Wallet, argumento, passe, posição e webhook usam transações explícitas, constraints e idempotência.

Contadores são projeções reconstruíveis. Feeds usam keyset pagination. Busca começa no PostgreSQL. Eventos destinados a jobs podem usar transactional outbox na mesma transação do domínio.

Interfaces de persistência são específicas, como `ReserveInkForArgument`, e não um `Repository[T]` genérico que vaza detalhes e multiplica boilerplate.

## 12. Reutilização sem duplicação

- regra de negócio existe uma vez no domínio ou caso de uso;
- transação é declarada na aplicação e implementada pelo adapter;
- validação sintática fica no transporte; invariantes ficam no domínio;
- mapeamentos repetitivos podem ser gerados, nunca escondidos por reflection em runtime;
- cross-cutting concerns usam middleware/adapters, não chamadas espalhadas;
- utilitários genéricos só existem após três usos reais coerentes;
- composição substitui herança e registries globais.

## 13. Caminho de escala

1. VPS única com edge cache e pool limitado.
2. aplicação stateless replicada; sessões e jobs continuam persistentes.
3. PostgreSQL em host dedicado com PITR e maior I/O.
4. réplicas para queries públicas comprovadamente read-heavy.
5. particionamento somente para tabelas cujo tamanho e padrão de acesso justifiquem.
6. Valkey ou broker apenas quando coordenação/throughput não couber no PostgreSQL medido.
7. extração de serviço somente para isolamento de carga, falha, segurança ou ownership.

O objetivo de atender milhões orienta statelessness, cache e contratos. Capacidade real é demonstrada por SLO, testes de carga e métricas; não é garantida pela forma do diagrama.
