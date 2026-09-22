# Documentação do Goyim Arena

Esta pasta contém a definição atual do produto. Os documentos distinguem deliberadamente:

- **Princípio:** compromisso duradouro, alterado apenas com justificativa pública forte.
- **Decisão:** regra adotada para a versão atual, passível de revisão documentada.
- **Hipótese:** suposição que precisa ser validada com pesquisa ou uso real.
- **Questão aberta:** decisão ainda não tomada.

Quando houver conflito, a ordem de precedência é:

1. [Constituição](CONSTITUTION.md);
2. [Regras de negócio](BUSINESS_RULES.md);
3. documento específico do tema;
4. [Visão do produto](PRODUCT.md);
5. materiais históricos.

## Produto

- [PRODUCT.md](PRODUCT.md) — problema, proposta de valor, público e posicionamento.
- [CONSTITUTION.md](CONSTITUTION.md) — compromissos fundamentais da plataforma.
- [BUSINESS_RULES.md](BUSINESS_RULES.md) — regras, estados, invariantes e casos-limite.
- [MVP.md](MVP.md) — escopo estrito e critérios de sucesso da primeira versão.
- [ROADMAP.md](ROADMAP.md) — sequência de validação e evolução.

## Operação e confiança

- [MONETIZATION.md](MONETIZATION.md) — INK, Arena Pass, assinatura e preços.
- [MODERATION.md](MODERATION.md) — política e processo de moderação.
- [PRIVACY.md](PRIVACY.md) — minimização, direitos e separação de dados.
- [TRANSPARENCY.md](TRANSPARENCY.md) — métricas públicas, histórico e exportações.
- [BUILD_IN_PUBLIC.md](BUILD_IN_PUBLIC.md) — como o projeto será construído publicamente.

## Tecnologia

- [STACK.md](STACK.md) — stack aprovada e política de versões.
- [ARCHITECTURE.md](ARCHITECTURE.md) — arquitetura inicial e limites dos módulos.
- [FRONTEND.md](FRONTEND.md) — componentes TypeScript nativos e CSS.
- [BACKEND.md](BACKEND.md) — domain, application, ports e adapters.
- [SCALABILITY.md](SCALABILITY.md) — caminho mensurável para alta escala.
- [SECURITY.md](SECURITY.md) — requisitos técnicos de segurança.
- [DEPLOYMENT.md](DEPLOYMENT.md) — topologia, ambientes, backup e evolução.
- [RUNBOOKS.md](RUNBOOKS.md) — alertas iniciais, thresholds e runbooks de incidente.
- [CI.md](CI.md) — verificação completa de release: gates exigidos, jobs, ações pinadas e orçamento do pipeline.
- [COMMITS.md](COMMITS.md) — Conventional Commits, scopes e versionamento.
- [ADRs](adr/README.md) — decisões arquiteturais e suas consequências.

## Aprendizado e governança

- [GOVERNANCE.md](GOVERNANCE.md) — decisões humanas do lançamento: o que está decidido, onde cada decisão é aplicada e o que ainda bloqueia o release (`make release-gate`).
- [METRICS.md](METRICS.md) — métricas de produto e guardrails.
- [RISKS_AND_ASSUMPTIONS.md](RISKS_AND_ASSUMPTIONS.md) — riscos, hipóteses e plano de validação.
- [DECISIONS.md](DECISIONS.md) — log de decisões de produto.
- [GLOSSARY.md](GLOSSARY.md) — vocabulário comum.

## Regra de manutenção

Toda mudança que altere incentivos, elegibilidade, contagem pública, moderação, preço, reputação ou privacidade deve atualizar os documentos afetados e acrescentar uma entrada em `DECISIONS.md` no mesmo conjunto de mudanças.
