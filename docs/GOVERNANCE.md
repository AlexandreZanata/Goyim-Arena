# Governança de lançamento

**Status:** decisões do titular do repositório; a metade aberta (abaixo) impede o release

**Objetivo:** registrar, em documento versionado, cada decisão humana que bloqueia um lançamento — e declarar em voz alta o que ainda **não** foi decidido, porque uma decisão que ninguém tomou não pode virar comportamento por omissão.

Este documento existe por causa da P20-T01, e ele responde às sete decisões que a fase exige: idade mínima, licença do repositório, contato legal/privacidade, política de retenção, mercados, termos de uso e canal de segurança. As perguntas estavam espalhadas por `PRIVACY.md` §9/§10, `BUSINESS_RULES.md` §12, `RISKS_AND_ASSUMPTIONS.md` §4, `DEPENDENCIES.md` §6 e `SECURITY.md` da raiz; a decisão é registrada aqui e os documentos originais apontam para cá, em vez de carregarem uma segunda cópia que envelhece.

## Como este documento é lido

- **decidida** — o proprietário decidiu; a entrada diz o que foi decidido, quando e **onde a decisão é aplicada**. Decisão sem lugar de aplicação é anotação, não decisão, e o auditor recusa;
- **bloqueio** — o proprietário ainda **não** decidiu; a entrada diz o que falta decidir, quem deve decidir e o que a decisão impede. Enquanto houver bloqueio, `make release-gate` sai com erro nomeando cada um: a lista abaixo não é aviso, é portão;
- **pendência** — passo que falta para *cumprir* uma decisão já tomada (entregar um arquivo, criar um endereço, concluir uma revisão). Pendência **nunca** substitui decisão: o auditor imprime as pendências e não as trata como item aberto.

O bloco JSON ao final é a metade executável do registro: `tools/governanceaudit` o lê, confere o esquema, exige as sete decisões da fase, e compara cada entrada com a sua seção de prosa — texto e bloco não podem discordar.

## Decisões

### age-minimum — idade mínima

**Estado:** decidida

**Decisão:** dezoito anos, em todos os mercados, sem exceção por mercado e sem fluxo para menores.

**Consequência:** não existe cadastro de menor, não existe consentimento parental a implementar e não existe fluxo que incentive a participação de menores. A plataforma declara o mínimo no cadastro e nos termos, e a verificação é responsabilidade de quem aceita os termos — o produto não coleta data de nascimento para checá-la.

**Aplicada em:** `docs/PRIVACY.md` §9 (o item deixa de ser questão aberta e passa a apontar para cá) e `docs/BUSINESS_RULES.md` §12 (a pergunta da idade mínima é respondida).

**Revisar quando:** um mercado exigir tratamento diferente para menores; a mudança é uma decisão nova registrada aqui, não um ajuste de texto.

### repository-license — licença do repositório

**Estado:** decidida

**Decisão:** AGPL-3.0, identificador SPDX `AGPL-3.0-only`.

**Consequência:** quem oferecer o serviço pela rede, modificado, precisa publicar o código dessas modificações. É a licença do copyleft forte e ela é compatível com construir em público: o produto continua comercial (assinatura), e a contrapartida é que a operação do serviço modificado fica aberta. Ela não afeta as licenças das dependências, que seguem a política de `docs/DEPENDENCIES.md` §6.

**Aplicada em:** esta decisão e a nota de `docs/DEPENDENCIES.md` §6, que passa a distinguir a licença do **código do projeto** da política de licenças de **dependências**.

**Pendências:** entregar o arquivo `LICENSE` com o texto canônico da AGPL-3.0 e a linha de copyright. A linha exige o nome do titular do direito autoral, que **não** é um dado que este repositório possa inventar: até ele ser informado pelo proprietário, o arquivo não entra, e a licença declarada aqui é a decisão, não o instrumento.

### data-subject-channel — contato legal e de privacidade

**Estado:** decidida

**Decisão:** os pedidos de titular (acesso, correção, exclusão, informação sobre tratamento) e o contato de privacidade chegam por um **alias de e-mail dedicado**, criado e mantido pelo proprietário. O endereço é preenchido quando o alias existir; nenhum endereço é inventado neste documento.

**Consequência:** existe um canal público e rastreável para o exercício de direitos, distinto do canal privado de segurança. Enquanto o alias não existir, o canal está decidido mas ainda não é alcançável — e é por isso que a criação dele é pendência com data-limite: antes de qualquer beta público.

**Aplicada em:** esta decisão e o checklist de `docs/PRIVACY.md` §10, que passa a nomear o canal como alias dedicado.

**Pendências:** criar o alias e publicá-lo no aviso de privacidade em pt-BR e en-US.

### retention-policy — política de retenção

**Estado:** decidida

**Decisão:** as janelas que **já rodam** no código passam a ser política aprovada, e não apenas implementação: `tokens` e `sessions` 30 dias após o término, `abuse_signals` 7 dias (anonimização), `exports` 24 horas, `referential_logs` (trilha administrativa) e `billing` retidos como evidência, sem prazo.

**Consequência:** mudar uma janela ou uma ação (apagar, anonimizar, reter) é mudança de política e exige decisão nova registrada aqui — o que `docs/PRIVACY.md` §5 já exigia, agora com a ratificação do proprietário. Retenção legal ativa (`app.retention_holds`) continua suspendendo a ação, e as classes continuam independentes.

**Aplicada em:** `internal/profiles/domain/retention.go` (a política executável), `docs/PRIVACY.md` §5 (a tabela e a ratificação) e os testes de limite de `internal/profiles` e `internal/platform/postgres`.

**Pendências:** a revisão jurídica do cronograma entra no checklist de pré-beta (`docs/PRIVACY.md` §10) e não altera a política em vigor até produzir uma decisão.

### launch-markets — mercados

**Estado:** decidida

**Decisão:** Brasil **e** internacional desde o beta, servindo português do Brasil e inglês dos Estados Unidos — o que o README já declarava como intenção e que passa a ser decisão do proprietário.

**Consequência:** o mapeamento de onde os dados são processados, os mecanismos de transferência internacional e as responsabilidades de cada fornecedor deixam de ser "antes de operar fora do Brasil" e passam a ser pré-requisito do próprio beta, porque o beta já atende fora. A lista de subprocessadores precisa ser publicada e as mudanças relevantes, avisadas.

**Aplicada em:** `README.md` (mercados iniciais), `docs/PRIVACY.md` §8 e §10.

**Pendências:** concluir o mapeamento de transferência internacional e publicar a lista de subprocessadores antes do beta.

### terms-of-use — termos de uso

**Estado:** bloqueio

**Bloqueio:** decidir se o beta público exige **termos de uso** e **aviso de privacidade** publicados nos dois idiomas — e quem os redige e sob que revisão jurídica. Hoje não existe `TERMS.md`; `docs/PRIVACY.md` §10 lista os dois como itens de pré-beta, sem decisão de escopo (redigir agora, contratar revisão, ou adiar e mantê-los como bloqueio com data).

**Quem decide:** titular do repositório.

**Impede:** o beta público. Um serviço com contas, cobrança e conteúdo publicado não entra em beta aberto sem termos que digam o que o usuário aceita e sem aviso de privacidade que diga o que é tratado e por quê.

### security-channel — canal de segurança

**Estado:** bloqueio

**Bloqueio:** confirmar o canal de reporte de vulnerabilidade e os prazos públicos. `SECURITY.md` na raiz já descreve o canal privado do GitHub (*Security → Report a vulnerability*) e deixa os prazos "para quando houver equipe e processo capazes de cumpri-los" — falta a decisão do proprietário sobre manter esse canal como o oficial do lançamento e sobre publicar (ou não) um compromisso de resposta.

**Quem decide:** titular do repositório.

**Impede:** o release. O checklist de release de `docs/SECURITY.md` §11 exige contato privado de segurança disponível, e um canal sem decisão explícita sobre prazos é um canal que ninguém sabe se pode cobrar.

## Registro executável

```json
{
  "decided_by": "titular do repositório",
  "recorded_at": "2026-09-22",
  "items": [
    {
      "id": "age-minimum",
      "status": "decided",
      "decision": "18 anos em todos os mercados, sem exceção por mercado e sem fluxo para menores",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §9 e docs/BUSINESS_RULES.md §12 apontam para esta decisão",
      "pending": []
    },
    {
      "id": "repository-license",
      "status": "decided",
      "decision": "AGPL-3.0, identificador SPDX AGPL-3.0-only",
      "decided_at": "2026-09-22",
      "enforced_by": "esta decisão e a nota de docs/DEPENDENCIES.md §6 sobre a licença do código do projeto",
      "pending": [
        "entregar LICENSE com o texto canônico da AGPL-3.0 e a linha de copyright, que exige o nome do titular do direito autoral informado pelo proprietário"
      ]
    },
    {
      "id": "data-subject-channel",
      "status": "decided",
      "decision": "pedidos de titular e contato de privacidade por alias de e-mail dedicado, criado e mantido pelo proprietário",
      "decided_at": "2026-09-22",
      "enforced_by": "esta decisão e o checklist de docs/PRIVACY.md §10, que nomeia o canal",
      "pending": [
        "criar o alias e publicá-lo no aviso de privacidade em pt-BR e en-US antes do beta público"
      ]
    },
    {
      "id": "retention-policy",
      "status": "decided",
      "decision": "ratificadas as janelas em vigor: tokens e sessions 30 dias, abuse_signals 7 dias, exports 24 horas, referential_logs e billing sem prazo",
      "decided_at": "2026-09-22",
      "enforced_by": "internal/profiles/domain/retention.go, docs/PRIVACY.md §5 e os testes de limite de internal/profiles e internal/platform/postgres",
      "pending": [
        "revisão jurídica do cronograma no checklist de pré-beta (docs/PRIVACY.md §10)"
      ]
    },
    {
      "id": "launch-markets",
      "status": "decided",
      "decision": "Brasil e internacional desde o beta, com pt-BR e en-US",
      "decided_at": "2026-09-22",
      "enforced_by": "README.md (mercados iniciais) e docs/PRIVACY.md §8 e §10",
      "pending": [
        "concluir o mapeamento de transferência internacional e publicar a lista de subprocessadores antes do beta"
      ]
    },
    {
      "id": "terms-of-use",
      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "decidir se o beta público exige termos de uso e aviso de privacidade publicados nos dois idiomas, quem os redige e sob que revisão jurídica",
        "owner": "titular do repositório",
        "blocks": "beta público"
      }
    },
    {
      "id": "security-channel",
      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "confirmar o canal oficial de reporte de vulnerabilidade (manter o canal privado do GitHub de SECURITY.md ou trocar) e decidir se prazos de resposta serão publicados",
        "owner": "titular do repositório",
        "blocks": "release"
      }
    }
  ]
}
```

## O portão

`make release-gate` roda `tools/governanceaudit` sobre este documento. Ele tem dois modos:

- `governanceaudit -check` — julga o documento (esquema, as sete decisões presentes, prosa de acordo com o bloco) e sai 0 mesmo com bloqueio aberto: é o modo dos testes da ferramenta;
- `governanceaudit` — o **portão de release**: além de julgar o documento, **falha** enquanto qualquer item estiver bloqueado, nomeando o que falta, quem deve e o que impede.

O alvo **não** entra em `make verify` de propósito: `verify` é o gate de integração de cada merge, e um merge não é um release. Ele é o que se roda antes de publicar, e é isso que a P20 verifica de ponta a ponta (`docs/RELEASE_CHECKLIST.md`).

Enquanto os dois bloqueios acima existirem, o portão sai vermelho — e é esse o estado correto: as decisões foram pedidas e ainda não foram tomadas.
