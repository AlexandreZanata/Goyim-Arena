# ADR-003 — PostgreSQL como única fonte persistente inicial

**Status:** aceito

**Data:** 2026-09-16

## Contexto

Wallet, publicação, posição e persuasão exigem consistência forte. Busca, jobs e sessões iniciais cabem no mesmo banco.

## Decisão

Usar PostgreSQL 18.x como fonte persistente. Acesso por pgx v5, queries com sqlc e migrations SQL. Full-text search, jobs e sessões permanecem no PostgreSQL no MVP.

## Alternativas

- Supabase: remove parte da operação, mas contraria a implantação autogerida escolhida.
- ORM: acelera CRUD simples, mas obscurece queries críticas e tipos do banco.
- Redis, broker e motor de busca separados: não há carga medida que justifique operação adicional.

## Consequências

- transações e backup têm uma fronteira clara;
- o banco concentra risco e exige monitoramento, tuning e restauração testada;
- projeções derivadas precisam ser reconstruíveis;
- novos stores exigem ADR e plano de consistência.

## Revisão

Reavaliar após otimização de queries e evidência de que uma responsabilidade excedeu o banco sem comprometer a fonte de verdade.
