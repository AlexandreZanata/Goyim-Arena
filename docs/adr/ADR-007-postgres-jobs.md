# ADR-007 — Jobs no PostgreSQL, sem Redis ou broker

**Status:** aceito

**Data:** 2026-09-16

## Contexto

Email, manutenção e projeções precisam de execução assíncrona, mas a escala inicial não justifica um sistema distribuído adicional.

## Decisão

Persistir jobs versionados no PostgreSQL. Worker Go usa lease, retry, idempotência e `FOR UPDATE SKIP LOCKED`. Server e worker compartilham o mesmo binário e módulos.

## Alternativas

- executar após a resposta sem persistência: perde trabalho em crash.
- Redis/Valkey: adiciona store e política de durabilidade.
- RabbitMQ/Kafka: capacidade desnecessária e maior custo operacional.

## Consequências

- enqueue pode participar da mesma transação do domínio;
- backups preservam jobs pendentes;
- jobs pesados podem competir com tráfego do banco e exigem limites;
- dashboard operacional precisa ser construído ou consultado com tooling seguro.

## Revisão

Adotar outro mecanismo apenas se contenção, throughput ou isolamento continuarem insuficientes após índices, lotes e workers ajustados.
