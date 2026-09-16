# Estratégia de escalabilidade

**Status:** direção arquitetural; não é promessa de capacidade

## 1. O que significa “milhões”

Milhões de contas cadastradas, visitantes mensais, usuários simultâneos e escritas por segundo são problemas diferentes. O projeto não publicará capacidade sem workload, hardware e teste reproduzível.

A arquitetura deve evitar reescrita desnecessária caso o produto alcance milhões de pessoas, mas cada estágio só é ativado por métrica.

## 2. Princípios

- cachear leitura pública na borda;
- manter aplicação stateless;
- limitar concorrência antes de esgotar banco;
- usar queries previsíveis e keyset pagination;
- separar fonte de verdade de projeções;
- mover trabalho não interativo para jobs idempotentes;
- escalar verticalmente antes de distribuir sem necessidade;
- preservar backpressure e graceful degradation.

## 3. SLOs iniciais propostos

Após o beta fornecer baseline, estabelecer:

- disponibilidade mensal por classe de rota;
- p95 e p99 de leitura pública com cache frio/quente;
- p95 de mutações críticas;
- taxa máxima de erro;
- atraso máximo de jobs e webhooks;
- RPO e RTO comprovados.

SLO sem medição e orçamento de erro não é compromisso válido.

## 4. Estágios

### Estágio 1 — VPS única

Cloudflare absorve leitura pública; Go, worker e PostgreSQL rodam com limites. Meta: validar produto e encontrar perfil real de carga.

### Estágio 2 — separar o banco

Quando I/O, memória, backup ou risco justificarem, PostgreSQL vai para host dedicado. App permanece stateless e ganha réplicas horizontais.

### Estágio 3 — leitura em escala

Melhorar cache hit, ETag, projeções e índices antes de réplica. Réplicas atendem somente queries tolerantes a atraso explicitamente marcadas.

### Estágio 4 — tabelas grandes

Particionar somente por padrão de acesso comprovado, geralmente tempo ou aggregate estável. Índices parciais e archival vêm antes de shard.

### Estágio 5 — coordenação distribuída

Adicionar Valkey, broker ou serviço extra apenas se Postgres jobs, locks e cache de borda forem insuficientes por evidência. Cada novo componente exige owner, backup, monitoramento e modo de falha.

### Estágio 6 — extração seletiva

Extrair serviço por isolamento real, não por previsão. Billing/webhooks ou busca podem ser candidatos futuros, mas continuam módulos enquanto isso reduzir risco.

## 5. Proteção do PostgreSQL

- pool pequeno por instância e limite global calculado;
- statement timeout e lock timeout por classe;
- queries observadas por `pg_stat_statements`;
- índices criados com hipótese e verificação;
- evitar N+1 e `SELECT *` em hot paths;
- batch com limites;
- keyset pagination;
- autovacuum e bloat monitorados;
- migrations expand/contract;
- projections rebuildable em vez de COUNT repetido.

## 6. Backpressure

- limites de payload e paginação;
- rate limits por ação e identidade;
- filas com tamanho e idade observados;
- worker com concorrência configurável;
- timeouts e circuit breaking nos adapters externos;
- rejeição controlada antes de OOM;
- funcionalidades não críticas degradam antes de wallet, posição e moderação.

## 7. Teste de capacidade

Cada release material mantém cenários k6 versionados. Resultados registram commit, hardware, dataset, cache, configuração e percentis. Testes incluem corrida de wallet, Arena viral, feed profundo, publicação, login e replay de webhook.

Uma decisão de escala só é aceita com gráfico, gargalo identificado, alternativa mais simples avaliada e teste posterior comprovando ganho.

## 8. Sinais de rearquitetura

- banco sustentadamente saturado após otimização;
- contenção entre workloads que exigem SLOs diferentes;
- deploy de um módulo coloca outros em risco recorrente;
- volume de dados excede manutenção e recovery objetivos;
- equipe não consegue evoluir módulos sem conflitos estruturais.

Número de contas ou linhas isoladamente não é sinal suficiente.
