# Arquitetura

**Status:** arquitetura inicial aprovada

**Estilo:** monólito modular orientado a domínio

## 1. Objetivos

- operar com margem em uma VPS de 16 GB;
- manter o conteúdo público rápido e indexável;
- preservar transações fortes para wallet, posições e atribuições;
- permitir auditoria e reconstrução de métricas;
- evitar serviços distribuídos antes de necessidade medida;
- manter caminho de saída de fornecedores.

## 2. Topologia inicial

```text
Internet
   │
   ▼
Cloudflare
DNS · CDN · WAF · Turnstile · rate limits de borda
   │
   ▼
Caddy
TLS · headers · reverse proxy · compressão
   │
   ├──────────────► Goyim Arena server (Go)
   │                 HTML · HTMX fragments · JSON público
   │                 auth · autorização · domínio · billing
   │
   └──────────────► Goyim Arena worker (mesma imagem Go)
                     email · jobs · manutenção assíncrona
                         │
                         ▼
                    PostgreSQL 18
              única fonte persistente inicial

Externos: Stripe · Resend · Sentry · PostHog · R2 futuro
```

Server e worker usam o mesmo código e comandos diferentes. Podem começar no mesmo host e ser separados sem dividir o domínio em microserviços.

## 3. Módulos

O processo é único, mas os módulos não acessam tabelas ou regras uns dos outros arbitrariamente.

- `identity`: contas, email, credenciais, sessões e recuperação.
- `profiles`: usernames, locale e perfil público.
- `arenas`: rascunho, publicação, fechamento, categoria e idioma.
- `positions`: posição inicial, atual e mudanças.
- `arguments`: argumentos, respostas, fontes e retirada.
- `persuasion`: elegibilidade, atribuições e métricas públicas.
- `wallet`: contas, lançamentos, buckets e consumo atômico.
- `billing`: produtos, checkout, assinatura e webhooks.
- `moderation`: denúncias, decisões, recursos e sanções.
- `transparency`: agregados públicos e exportações versionadas.
- `audit`: eventos administrativos e integridade.
- `notifications`: emails transacionais e preferências futuras.
- `jobs`: fila persistente e políticas de retry.

Cada módulo possui tipos de domínio, serviço de aplicação, queries e interfaces próprias. Dependências cruzadas passam por serviços explícitos; ciclos são proibidos.

## 4. Organização futura do repositório

```text
cmd/
  arena/          servidor HTTP, worker e comandos operacionais
internal/
  identity/
  profiles/
  arenas/
  positions/
  arguments/
  persuasion/
  wallet/
  billing/
  moderation/
  transparency/
  audit/
  jobs/
  platform/       postgres, email, observabilidade e clock
web/
  components/     arquivos templ
  assets/         CSS, JS e imagens versionadas
db/
  migrations/
  queries/        SQL de entrada do sqlc
  generated/      código gerado; política será definida antes do código
infra/
  compose/
  caddy/
  postgres/
docs/
  adr/
tests/
  e2e/
  load/
```

Isto não é um monorepo de múltiplos produtos: é um único módulo Go e uma única unidade de deploy, com infraestrutura e documentação no mesmo repositório.

## 5. Separação entre conteúdo público e privado

O HTML público nunca contém email, saldo, sessão, posição individual ou autoria de atribuição. Conteúdo personalizado usa rotas sem cache.

- `/d/:slug`: statement, contexto e conteúdo público; pode ser cacheado.
- fragmentos públicos de argumentos e agregados: cacheáveis com TTL curto.
- `/me/*`, `/wallet/*`, `/admin/*` e respostas autenticadas: `private, no-store`.
- toda resposta com `Set-Cookie`: não elegível para cache compartilhado.

O bloqueio do agregado antes da escolha é uma regra de experiência, não um segredo de segurança. Argumentos continuam públicos e indexáveis. Metadados e previews não exibem o resultado agregado.

## 6. Cache e consistência

- Assets com hash: cache longo e imutável.
- Páginas e fragmentos públicos: TTL curto, inicialmente na ordem de dezenas de segundos.
- Dados privados, wallet, checkout e admin: nunca em cache compartilhado.
- Após escrita, a resposta ao autor mostra estado confirmado diretamente da origem.
- Purge de cache é otimização; correção não pode depender apenas de purge remoto.
- Conteúdo removido por risco grave exige invalidação prioritária e resposta segura na origem.

As regras do Cloudflare devem excluir cookies e rotas privadas explicitamente. “Cache everything” global é proibido.

## 7. Persistência e transações

PostgreSQL é a fonte de verdade. Wallet, publicação de argumento, consumo de passe, mudança de posição e webhook usam transações explícitas e idempotência.

Contadores como `arguments_count` ou `minds_changed_count` são projeções reconstruíveis. Os eventos normalizados continuam sendo a autoridade. Atualização pode ser transacional quando barata ou por job idempotente quando eventual.

Feeds usam keyset pagination. `OFFSET` profundo é proibido em fluxos de produção. Busca começa com full-text search do PostgreSQL.

## 8. Autorização e acesso ao banco

O browser nunca acessa PostgreSQL. Toda autorização ocorre no servidor Go.

Papéis separados:

- migrator: altera schema; não é usado no runtime;
- app: somente permissões necessárias à aplicação;
- backup: somente permissões de backup;
- observability: leitura limitada de métricas.

RLS universal não é requisito porque nenhuma tabela é exposta diretamente. Pode ser adotado como defesa adicional em dados específicos, com testes, sem substituir autorização de aplicação.

## 9. Jobs

Jobs ficam em tabela própria com tipo, payload versionado, estado, tentativas, agendamento, lease e chave de idempotência. Workers obtêm lotes com `FOR UPDATE SKIP LOCKED`.

Payload não armazena segredos nem cópias desnecessárias de dados pessoais. Dead letters são estados consultáveis, não outra infraestrutura.

## 10. Caminho de evolução

1. VPS única: Caddy, app, worker e PostgreSQL.
2. Banco em host dedicado quando I/O, risco operacional ou memória justificarem.
3. Múltiplas instâncias stateless da aplicação atrás de proxy.
4. Valkey apenas quando coordenação distribuída ou cache compartilhado tiver ganho medido.
5. Réplica PostgreSQL para leituras somente após queries e cache estarem otimizados.

Dividir um módulo em serviço independente exige ownership, gargalo ou isolamento de risco demonstrável.
