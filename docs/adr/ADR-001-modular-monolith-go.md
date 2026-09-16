# ADR-001 — Monólito modular em Go

**Status:** aceito

**Data:** 2026-09-16

## Contexto

O sistema concentra HTTP, regras transacionais, PostgreSQL e renderização de páginas. A equipe inicial e uma VPS favorecem uma única unidade operacional.

## Decisão

Usar Go 1.27.x em um único módulo e deploy, separado internamente por domínios. O mesmo artefato fornece comandos de servidor, worker e operação.

## Alternativas

- Next.js e Supabase: produtivos, mas adicionam runtime e acoplamento gerenciado que não são necessários à estratégia da VPS.
- Microserviços: aumentam coordenação, deploy e observabilidade sem isolamento exigido pelo MVP.

## Consequências

- baixo consumo e deploy simples;
- transações locais e refactors mais fáceis;
- limites modulares precisam de revisão, pois não são impostos pela rede;
- frontend rico exigirá disciplina com HTML e interatividade progressiva.

## Revisão

Reavaliar somente quando um módulo exigir escala, disponibilidade, segurança ou ownership independentes demonstráveis.
