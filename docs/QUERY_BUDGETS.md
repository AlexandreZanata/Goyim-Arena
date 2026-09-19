# Orçamentos de queries

**Status:** baseline reproduzível de teste; não é promessa de capacidade de produção.

P17-T04 adiciona contagem e latência acumulada de queries por contexto usando
`internal/platform/dbbudget`. O tracer pgx registra cada query iniciada e sua
duração quando um `Tracker` está presente no contexto. O tracker é concorrente,
mas não altera a execução nem expõe SQL ou parâmetros.

## Budgets atuais

| Hot path | Máximo de queries | Latência acumulada máxima | Escopo |
| --- | ---: | ---: | --- |
| feed público | 1 | 2s | uma página, incluindo lookahead |
| Arena pública | 1 | 2s | leitura do documento |
| profile público | 1 | 2s | projeção pública |
| wallet statement/balance | 1 | 2s | uma leitura do proprietário |

Os limites são budgets de regressão para o ambiente de teste. Eles devem ser
reavaliados somente com um novo dataset sintético, hardware/configuração
registrados e evidência de carga; não representam p95/p99 de produção.

## Evidência reproduzível

`go test ./internal/platform/dbbudget -run TestHotPathBudgetsCountRealPostgreSQLQueries -count=1`
cria um banco PostgreSQL descartável, aplica as migrations, insere somente uma
conta, um profile, uma wallet e uma Arena sintéticos, e falha se qualquer hot
path exceder uma query ou o limite de latência. Os testes unitários também
falham separadamente quando a contagem ou a latência excede o budget.

Para guardar um plano de uma hipótese de índice, execute somente sobre o banco
sintético descartável:

```sql
EXPLAIN (ANALYZE, BUFFERS, COSTS OFF)
SELECT id, slug, statement, category, language, status, published_at
FROM app.arenas
WHERE status IN ('published', 'closed', 'restricted')
ORDER BY published_at DESC, id DESC
LIMIT 21;
```

O plano deve ser salvo junto do relatório da execução, com o commit, hardware,
quantidade de linhas sintéticas e configuração do PostgreSQL. Nenhum plano de
produção ou dado real deve ser versionado. A validação de regressão permanece
no teste de budget; `EXPLAIN` não é usado para mascarar ou aceitar N+1.

## Resultado desta microtarefa

- A instrumentação é opt-in por contexto e não adiciona headers públicos.
- O caminho de erro não reduz o budget nem ignora queries.
- As quatro classes possuem teste explícito; qualquer nova query nessas
  operações excede o limite e falha o gate.
- Nenhum N+1 foi encontrado nos caminhos medidos; correção de N+1 futuro deve
  ser uma mudança isolada com seu próprio teste e plano sintético.
