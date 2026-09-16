# ADR-002 — HTML server-side com templ e HTMX

**Status:** substituído pelo [ADR-009](ADR-009-native-typescript-frontend.md)

**Data:** 2026-09-16

> Registro histórico. A decisão abaixo não representa mais a stack atual.

## Contexto

A maior parte do produto é leitura pública, indexável e baseada em formulários. Uma SPA adicionaria hidratação e estado cliente sem benefício proporcional.

## Decisão

Renderizar HTML no Go com templ. Usar HTMX 2.x para fragmentos e JavaScript vanilla para comportamentos pequenos. Tailwind CSS compõe o design system.

## Alternativas

- React/Next.js: ecossistema amplo, mas maior custo de runtime, build e duplicação de estado.
- HTML sem HTMX: simples, porém piora interações como filtros, formulários e paginação incremental.
- Alpine.js desde o início: útil, mas desnecessário antes de aparecer estado local repetitivo.

## Consequências

- páginas rápidas, SEO direto e menos JavaScript;
- server e fragments precisam compartilhar contratos de renderização;
- acessibilidade funciona primeiro sem depender de script;
- componentes muito interativos podem exigir solução específica futura.

## Revisão

Reavaliar Alpine.js ou componente isolado quando JavaScript vanilla repetido se tornar custo mensurável. Não migrar toda a aplicação por um único widget.
