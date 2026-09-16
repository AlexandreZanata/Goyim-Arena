# Build in public

**Status:** processo inicial

## 1. O que será público

- repositório e histórico de mudanças;
- documentação do produto;
- roadmap e estado dos itens;
- decisões de produto e, futuramente, de arquitetura;
- regras de ranking, contagem e moderação;
- changelog de versões;
- métricas agregadas de transparência;
- incidentes e aprendizados quando a divulgação for segura.

## 2. O que não será público

- segredos e credenciais;
- dados pessoais;
- denúncias e evidências identificáveis;
- vulnerabilidades ainda exploráveis;
- detalhes antifraude que facilitem evasão;
- contratos ou informações de terceiros sob confidencialidade legítima.

“Build in public” não é licença para comprometer usuários.

## 3. Estados do roadmap

- **Proposed:** problema e alternativa em discussão; sem promessa de entrega.
- **Building:** trabalho ativo com escopo definido.
- **Released:** disponível no ambiente indicado e documentado.
- **Rejected:** não será seguido agora, com motivo e possibilidade de revisão.

Itens também podem retornar a Proposed se novas evidências invalidarem o escopo.

## 4. Registro de decisão

Toda decisão relevante deve conter contexto, decisão, alternativas, consequências, evidência e data de revisão. Decisões de produto ficam em `DECISIONS.md`; decisões técnicas futuras usarão ADRs em `docs/adr`.

## 5. Cadência sugerida

- atualização curta semanal: aprendizado, métrica e próximo experimento;
- revisão mensal do roadmap e riscos;
- changelog a cada release;
- relatório de transparência quando houver volume suficiente;
- postmortem para incidente material.

Não transformar produção de conteúdo sobre o projeto em prioridade acima de conversar com usuários e melhorar o produto.

## 6. Linguagem das comunicações

Separar sempre:

- fatos observados;
- interpretação;
- hipótese;
- decisão;
- promessa.

Evitar apresentar cadastro, impressão ou engajamento bruto como validação do problema. Publicar também resultados negativos e mudanças de direção.

## 7. Contribuição externa

Propostas devem descrever problema, impacto, alternativa e teste possível. Relatos de segurança devem usar canal privado, nunca issue pública enquanto houver risco ativo.
