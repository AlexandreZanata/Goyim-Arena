# ADR-008 — Autorização server-side sem RLS universal

**Status:** aceito

**Data:** 2026-09-16

## Contexto

Na proposta anterior com backend exposto, RLS em todas as tabelas era fronteira necessária. Na arquitetura escolhida, browser e clientes nunca conectam ao PostgreSQL.

## Decisão

Aplicar autorização em serviços Go, com testes por ação e recurso, e limitar o papel de runtime por grants. RLS não será universal no MVP; pode proteger conjuntos específicos como defesa em profundidade.

## Alternativas

- RLS em toda tabela: defesa adicional, mas exige contexto por conexão e testes complexos, sem substituir regras de aplicação.
- acesso direto do browser ao banco: rejeitado.

## Consequências

- regras de autorização permanecem próximas ao domínio;
- falha na aplicação não é automaticamente contida por RLS;
- testes negativos e revisão de grants tornam-se requisitos de release;
- adoção seletiva futura continua possível.

## Revisão

Reavaliar ao introduzir acesso de terceiros, múltiplos tenants organizacionais ou dados cujo impacto justifique duas fronteiras independentes.
