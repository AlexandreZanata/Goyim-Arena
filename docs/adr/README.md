# Architecture Decision Records

Esta pasta registra as decisões técnicas do Goyim Arena. ADRs aceitos não são reescritos para esconder a evolução: mudanças posteriores criam um novo ADR que substitui o anterior.

Cada decisão arquitetural relevante recebe um arquivo numerado contendo:

- status;
- contexto;
- forças de decisão;
- alternativas consideradas;
- decisão;
- consequências positivas e negativas;
- evidências e data de revisão.

## Índice

- [ADR-001 — Monólito modular em Go](ADR-001-modular-monolith-go.md)
- [ADR-002 — HTML server-side com templ e HTMX](ADR-002-server-rendered-html.md)
- [ADR-003 — PostgreSQL como única fonte persistente](ADR-003-postgresql-system-of-record.md)
- [ADR-004 — VPS com Docker Compose e Caddy](ADR-004-vps-docker-compose-caddy.md)
- [ADR-005 — Sessões opacas em vez de JWT no browser](ADR-005-opaque-sessions.md)
- [ADR-006 — Cloudflare na borda e cache público](ADR-006-cloudflare-edge-cache.md)
- [ADR-007 — Jobs no PostgreSQL, sem Redis ou broker](ADR-007-postgres-jobs.md)
- [ADR-008 — Autorização server-side sem RLS universal](ADR-008-server-side-authorization.md)
