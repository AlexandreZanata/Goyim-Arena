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

## 4. Runbook de decisão por evidência

Nenhuma etapa abaixo é ativada por volume de contas, previsão de marketing ou uma
métrica isolada. O gatilho precisa apontar para um baseline reproduzível (commit,
hardware, dataset, cache, configuração, p50/p95/p99, erro, CPU, memória, I/O,
conexões, locks e cache hit) e para uma regressão ou orçamento de erro observado.

Cada mudança tem uma saída conservadora: manter a topologia atual, comparar o
resultado com o baseline e reverter se o teste de sucesso não confirmar o ganho.

| Etapa | Métricas e gatilho observável | Risco principal | Rollback | Teste de sucesso |
|---|---|---|---|---|
| VPS única → banco dedicado | I/O, memória, conexões, locks ou manutenção do PostgreSQL consomem repetidamente o orçamento medido no baseline, mesmo após queries, pool e índices terem sido revisados. | Custo, rede e uma nova fronteira de backup/readiness. | Voltar a apontar o app para o banco anterior enquanto o tráfego permanece bloqueado para a nova origem; não remover dados sem backup verificado. | Reexecutar o mesmo cenário k6 e o exercício de restauração; confirmar que p95/p99, erros, locks e RPO/RTO não pioram. |
| Primário → réplicas de leitura | A carga de leituras tolerantes a atraso é o gargalo dominante e existe uma política explícita de frescor para cada query candidata; nenhuma réplica é usada para wallet, posição, moderação ou estado crítico. | Lag servir dados antigos ou uma falha de replicação esconder uma escrita. | Remover a rota de leitura da réplica e voltar ao primário; preservar o monitoramento de lag e o replay da configuração anterior. | Cenários de leitura cold/hot mantêm os percentis e o erro do baseline, o lag fica dentro do orçamento de frescor e leituras críticas continuam no primário. |
| Tabelas grandes → particionamento | Plano, bloat, autovacuum, manutenção ou latência degradam em uma tabela cujo padrão de acesso e chave de partição foram comprovados por `pg_stat_statements` e `EXPLAIN (ANALYZE, BUFFERS)`. | Locks, partições órfãs, consultas que deixam de usar pruning e migração difícil de desfazer. | Usar expand/contract: manter a tabela antiga e o caminho de leitura anterior até a validação; interromper a migração sem apagar a fonte. | Repetir o plano e o workload sintético antes/depois, confirmar ausência de full scan inesperado, integridade dos dados e conclusão/rollback da migração em banco descartável. |
| PostgreSQL jobs/cache → Valkey ou broker | Jobs, locks ou cache de borda continuam fora do orçamento após tuning, backpressure, índices e projeções; o relatório identifica a coordenação específica que saturou. | Durabilidade, duplicação, ordering, operação e nova superfície de indisponibilidade. | Manter PostgreSQL como fonte de verdade e desativar o consumidor novo; reprocessar a fila por idempotência, sem descartar jobs. | Testar crash/restart, duplicação, ordenação, atraso e recovery com o mesmo dataset; confirmar que wallet, posição e moderação não dependem do componente novo para consistência. |
| Monólito → serviço extraído | Um módulo tem fronteira de dados, owner, SLO, carga e modo de falha demonstrados; a contenção causa impacto mensurável que cache, queries, réplica e worker não resolveram. | Complexidade distribuída, contratos divergentes e falha de rede entre módulos. | Manter o caminho no monólito e desativar o consumidor externo por feature/configuração; reverter sem migração destrutiva. | Teste de contrato, replay de eventos, carga do módulo e falha de rede passam; p95/p99, erros e recuperação são iguais ou melhores que o baseline. |

### Checklist obrigatório antes de avançar

1. Anexar o relatório do baseline e o gráfico que identifica o gargalo.
2. Registrar a alternativa mais simples tentada e por que ela não bastou.
3. Definir owner, backup, monitoramento, rollback e período de observação.
4. Reexecutar o mesmo workload sintético após a mudança; não comparar testes com datasets ou hardware diferentes.
5. Abortar a etapa se qualquer rota crítica depender de consistência eventual sem contrato explícito.

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
