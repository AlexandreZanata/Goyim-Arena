# ADR-009 — Frontend TypeScript nativo e CSS nativo

**Status:** aceito; substitui ADR-002

**Data:** 2026-09-16

## Contexto

O projeto quer componentes profissionais sem dependência de frameworks ou bibliotecas no browser. TypeScript 7 está estável e o browser moderno oferece Custom Elements, módulos ESM, fetch e CSS avançado.

## Decisão

Usar TypeScript 7 compilado para ESM, Web Components/Custom Elements e CSS nativo. O browser recebe zero pacotes de terceiros. O único pacote de build do frontend é o compilador oficial TypeScript. Go pode entregar HTML semântico com `html/template` para SEO e progressive enhancement.

## Alternativas

- templ + HTMX + Tailwind: decisão anterior, substituída por adicionar runtimes e convenções externas ao frontend.
- framework SPA: oferece ecossistema amplo, mas aumenta dependência, payload e acoplamento ao framework.
- JavaScript sem tipos: reduz toolchain, porém enfraquece contratos e refactors em escala.
- Shadow DOM em tudo: isolamento forte, mas dificulta composição, forms, estilos e acessibilidade; será seletivo.

## Consequências

- supply chain e runtime do browser muito menores;
- componentes dependem de padrões web duráveis;
- a equipe assume responsabilidade por lifecycle, acessibilidade e primitives que frameworks normalmente fornecem;
- sem bundler, a granularidade de módulos e número de requests precisam de orçamento e medição;
- testes em browser real são obrigatórios.

## Guardrails

- `strict` e flags adicionais de segurança no TypeScript;
- `innerHTML` proibido para dados dinâmicos;
- CSS por camadas, tokens e escopo explícito;
- dependência frontend nova exige ADR;
- compatibilidade definida por browser policy, não por transpilation infinita.

## Revisão

Reavaliar somente quando uma capacidade concreta não puder ser entregue com a plataforma nativa dentro dos SLOs e custo de manutenção aceitos.
