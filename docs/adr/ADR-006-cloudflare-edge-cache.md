# ADR-006 — Cloudflare na borda e cache público

**Status:** aceito

**Data:** 2026-09-16

## Contexto

A maioria das leituras é pública e pode se tornar viral, enquanto a origem é uma única VPS. Dados privados não podem vazar por cache compartilhado.

## Decisão

Usar Cloudflare para DNS, CDN, WAF, Turnstile e cache seletivo. Cachear somente assets, páginas e fragmentos inequivocamente públicos; rotas pessoais e administrativas usam `no-store`.

## Alternativas

- servir tudo da origem: operacionalmente simples, mas desperdiça a natureza cacheável do produto.
- personalizar a página inteira: mistura cache e sessão, aumentando risco e carga.
- cache de HTML global: rejeitado pelo risco de armazenar respostas dinâmicas ou cookies.

## Consequências

- leituras podem escalar sem atingir a origem na mesma proporção;
- conteúdo público aceita consistência eventual curta;
- regras e headers de cache viram superfície de segurança testada;
- origem deve ser protegida para impedir bypass do edge.

## Revisão

Reavaliar TTLs e estratégia após métricas de hit ratio, purges, conteúdo removido e tráfego real.
