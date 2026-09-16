# ADR-005 — Sessões opacas em vez de JWT no browser

**Status:** aceito

**Data:** 2026-09-16

## Contexto

O produto web é server-rendered e precisa revogar sessões, proteger ações sensíveis e manter cookies simples.

## Decisão

Usar token de sessão aleatório em cookie seguro. Persistir somente seu hash no PostgreSQL por meio de uma biblioteca de sessão madura. Senhas usam Argon2id; tokens de email também são opacos e de uso único.

## Alternativas

- JWT no browser: útil em sistemas distribuídos, mas revogação e rotação adicionam complexidade sem benefício aqui.
- Provedor de identidade externo: reduz implementação, porém adiciona dependência e custo; pode ser reavaliado se requisitos crescerem.
- Keycloak: poderoso, mas operacionalmente excessivo para o MVP.

## Consequências

- revogação e listagem de sessões são diretas;
- cada autenticação consulta storage, aceitável no PostgreSQL inicial;
- fluxos de senha, email e MFA ainda exigem implementação e testes rigorosos;
- administradores precisam de MFA antes do beta.

## Revisão

Reavaliar identidade gerenciada ou serviço dedicado caso autenticação social, SSO empresarial ou requisitos regulatórios mudem o problema.
