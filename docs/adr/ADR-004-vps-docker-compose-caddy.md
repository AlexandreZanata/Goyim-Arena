# ADR-004 — VPS com Docker Compose e Caddy

**Status:** aceito

**Data:** 2026-09-16

## Contexto

Há uma VPS de 16 GB e prioridade de baixo custo. O sistema precisa ser reproduzível publicamente sem uma plataforma orquestradora complexa.

## Decisão

Executar Caddy, aplicação, worker e PostgreSQL com Docker Compose. Caddy termina TLS e atua como reverse proxy. Entre Cloudflare e origem, usar TLS `Full (strict)` com certificado de origem ou ACME DNS-01. Produção usa imagens imutáveis geradas pelo CI.

## Alternativas

- systemd com binário nativo: menor camada, mas ambiente menos reproduzível e mais configuração do host.
- Kubernetes: capacidade e complexidade muito acima da necessidade.
- PaaS: operação simples, porém custo e menor portabilidade para o objetivo atual.

## Consequências

- deploy e rollback de aplicação são simples;
- volumes, limites, health checks e logs precisam ser configurados explicitamente;
- Compose não oferece alta disponibilidade sozinho;
- PostgreSQL em container exige a mesma disciplina de backup e disco que uma instalação nativa.

## Revisão

Reavaliar quando houver múltiplos hosts, requisito real de alta disponibilidade ou carga que peça banco dedicado.
