# Arquitetura do backend

**Status:** padrão obrigatório

**Objetivo:** regras reutilizáveis, adapters substituíveis e evolução sem duplicação

## 1. Arquitetura

O backend usa monólito modular com ports and adapters. Isso preserva transações e simplicidade operacional enquanto impede que regras de negócio dependam de HTTP, banco ou fornecedores.

```text
Inbound adapters → Application → Domain
                         ↓
                  Outbound ports
                         ↓
                  Outbound adapters
```

## 2. Domain

Contém somente:

- entidades com identidade e comportamento;
- value objects validados;
- políticas de domínio;
- eventos de domínio;
- erros estáveis;
- invariantes que precisam valer em qualquer entrada.

Não contém struct de JSON, SQL tag, status HTTP, logger, client ou variável de ambiente.

## 3. Application

Cada caso de uso possui command/query, dependências mínimas, método de execução e resultado explícito. Ele:

- autoriza a intenção;
- abre a fronteira transacional necessária;
- carrega estado por ports específicos;
- chama comportamento de domínio;
- persiste resultado;
- registra evento/outbox quando aplicável;
- retorna dado independente de transporte.

Handlers não implementam regra de negócio. Workers e CLI chamam o mesmo caso de uso.

## 4. Ports

Ports pertencem ao módulo consumidor e descrevem capacidade, não tecnologia.

Preferir:

```text
LoadArenaForArgument
ReserveInk
AppendArgument
EnqueueNotification
```

Evitar:

```text
Repository[T]
Save(any)
Utils
Manager
ServiceFactory
```

Interfaces pequenas reduzem mocks, boilerplate e acoplamento. Um adapter pode implementar várias interfaces sem expor isso ao domínio.

## 5. Adapters

- HTTP traduz protocolo, autenticação, validação sintática e representação.
- PostgreSQL concentra SQL, pgx, sqlc, locks e erros do driver.
- Stripe concentra SDK, assinatura e mapeamento de eventos.
- Resend concentra protocolo de email.
- Observabilidade concentra Sentry, PostHog e métricas.
- Clock, IDs e randomness são injetáveis onde determinismo importa.

Tipos de fornecedor nunca atravessam o port. Cada adapter possui testes de integração e contrato.

## 6. Transações

O caso de uso define a unidade atômica; o adapter PostgreSQL fornece a execução. Wallet e publicação operam na mesma transação sem depender de transação distribuída.

Não abrir transação em middleware genérico para toda requisição. Não executar chamada de rede longa segurando lock. Efeitos externos usam outbox/job após commit quando possível.

## 7. APIs

- contrato OpenAPI versionado no repositório;
- Problem Details para erros HTTP;
- validação estrutural no adapter e invariantes no domínio;
- cursor opaco para paginação;
- idempotency key para pagamentos e comandos repetíveis;
- ETag/conditional request para conteúdo público;
- limites explícitos de payload, tempo e paginação;
- nenhuma entidade interna serializada diretamente.

## 8. Eventos

Eventos de domínio descrevem fato passado. Integrações assíncronas usam outbox transacional e consumidor idempotente. A tabela de jobs é transporte interno, não parte do domínio.

Eventos possuem nome, versão, ID, aggregate ID e timestamp. Payload mínimo reduz acoplamento e exposição de dados.

## 9. Erros

Erros são classificados:

- domínio: regra violada;
- aplicação: autorização, conflito ou pré-condição;
- infraestrutura: indisponibilidade, timeout ou corrupção;
- transporte: request inválida.

Somente adapter HTTP decide status e mensagem pública. Causa interna permanece em log redigido e correlação.

## 10. Redução de repetição

- geradores somente para código mecânico, nunca regra;
- sqlc remove scan repetitivo preservando SQL;
- tooling Go interno gera tipos TypeScript do contrato sem adicionar pacote ao frontend;
- middleware concentra autenticação, CSRF, request ID e limites;
- helpers só aparecem após padrão comprovado;
- composition root elimina construção espalhada;
- packages pequenos e coesos substituem pastas genéricas.

DRY não permite criar abstração prematura. Duas linhas parecidas com motivos diferentes podem permanecer separadas.

## 11. Testes

- domínio: testes unitários e property/fuzz tests de invariantes;
- aplicação: casos de uso com fakes manuais pequenos;
- PostgreSQL: integração em banco real e concorrência;
- adapters externos: contract tests e fixtures sanitizadas;
- HTTP: contrato, autorização, cache e Problem Details;
- arquitetura: teste que impede imports na direção errada;
- carga: cenários de leitura, escrita e contenção.

## 12. Critérios para extrair serviço

Um módulo só vira serviço quando houver ao menos uma razão demonstrável:

- escala independente material;
- falha precisa ser isolada;
- limite regulatório ou de segurança;
- equipe com ownership e cadência independentes;
- tecnologia inevitavelmente diferente.

Antes disso, a separação lógica permanece dentro do monólito.
