# ADR-010 — Backend API-first com ports and adapters

**Status:** aceito; detalha ADR-001

**Data:** 2026-09-16

## Contexto

Web, worker, CLI e clientes futuros devem reutilizar as mesmas regras. Handlers e integrações não podem duplicar domínio nem prender o sistema a PostgreSQL ou fornecedores.

## Decisão

Separar cada módulo em domain, application e adapters. Casos de uso definem ports pequenos; adapters implementam HTTP, PostgreSQL e integrações. OpenAPI 3.1 versiona o contrato HTTP. O composition root é o único local que conhece implementações concretas.

## Alternativas

- handlers chamando SQL diretamente: menos arquivos inicialmente, mas duplica autorização, transação e regras.
- arquitetura por camadas técnicas globais: cria módulos gigantes de services/repositories e dependências cruzadas.
- microserviços: impõem rede e consistência distribuída antes de existir necessidade.
- repositories genéricos: reduzem nomes, mas vazam modelo de persistência e escondem intenção.

## Consequências

- regras podem ser chamadas por qualquer inbound adapter;
- fornecedores tornam-se substituíveis nos limites definidos;
- há mais tipos de fronteira e mapeamento explícito;
- geração pode remover boilerplate mecânico, mas não regra;
- testes de arquitetura precisam impedir imports invertidos.

## Regra de qualidade

Domínio e aplicação não importam packages de adapter. Nenhum tipo de Stripe, pgx, sqlc, HTTP ou analytics atravessa um port.

## Revisão

Reavaliar a granularidade dos módulos após fluxos reais; preservar a direção de dependência mesmo se a estrutura de pastas evoluir.
