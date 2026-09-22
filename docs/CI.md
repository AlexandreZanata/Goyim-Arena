# Verificação de release no CI

**Status:** entregue na P19-T08. O CI verifica; ele **não** publica, não faz deploy e não tem credencial de produção.

Este documento é a superfície do pipeline: quais gates um merge exige, em que job cada um roda, o que o `tools/ciaudit` recusa, quanto tempo a verificação pode levar e o que ficou deliberadamente de fora. Onde ele afirma um número, o número foi medido no commit que o escreveu.

## 1. O que um merge exige

Dois workflows, dez jobs:

| Workflow | Job | Gates | Ferramentas que o job instala |
| --- | --- | --- | --- |
| `verify` | `foundation` | `make verify` — formatação, drift dos artefatos gerados, unit, integração PostgreSQL, race selecionado, migrations, contrato OpenAPI, segurança, build do frontend, `tsc` estrito, medição do frontend, auditoria de i18n, auditoria de rastreabilidade dos requisitos, auditoria do próprio CI | Go, Node, sqlc, PostgreSQL 18.4 (serviço) |
| `verify` | `browser` | `make test-e2e` — jornadas críticas em Chromium | Go, Node, Playwright (pinado em `tools/e2e`), PostgreSQL 18.4 (serviço) |
| `verify` | `image` | `make image-verify` e o scan da imagem construída | Go, Docker, trivy (ação) |
| `verify` | `ingress` | `make caddy-verify` | Go, Docker, openssl |
| `verify` | `stack` | `make compose-verify` | Go, Docker, openssl |
| `verify` | `backup` | `make backup-verify` | Go, Docker, openssl |
| `verify` | `deploy` | `make deploy-verify` | Go, Docker, openssl |
| `supply-chain` | `dependency-review` | revisão de dependências alteradas (`fail-on-severity: high`) | — |
| `supply-chain` | `source-scans` | `make vuln` (`govulncheck`), `npm audit --audit-level=high`, gitleaks | Go, Node, govulncheck (versão fixada) |
| `supply-chain` | `image-scan` | scan da imagem do PostgreSQL que o compose fixa | trivy (ação) |

A verificação completa é a **união** desses gates. Nenhum deles está copiado dentro de um workflow: cada job chama o alvo do Makefile, e é por isso que o CI não pode divergir do gate que um operador roda à mão.

## 2. `tools/ciaudit` — o gate do gate

`make audit-ci` roda dentro de `make verify` e lê os dois workflows e o `Makefile` como eles são. Ele recusa:

| Regra | O que ela impede |
| --- | --- |
| `gates-wired` | um gate que a fase exige e que nenhum job alcança — inclusive o gate agregado, que pode ser substituído por uma de suas partes sem que o resto mude |
| `make-targets-exist` | um job que invoca `make x` e um Makefile que não declara `x` |
| `actions-pinned` | ação de terceiro presa por tag em vez de commit de 40 caracteres, ou sem o comentário que diz a qual tag aquele SHA aponta |
| `permissions-minimal` | token com escrita (`contents: write`, `read-all`, `write-all`, escopo que não seja `read`/`none`) |
| `failure-never-masked` | passo que engole a própria falha: `\|\| true`, `\|\| exit 0`, `set +e`, `continue-on-error`, gate com `if: always()`/`failure()` |
| `database-service` | job que roda um gate que abre PostgreSQL e não declara o serviço `postgres`, não define `ARENA_DATABASE_URL`, ou aponta para uma porta que o serviço não publica |
| `draft-skip-without-reduction` | PR em rascunho que deixa de ser adiado e passa a ser **aprovado** (job sem a condição de rascunho, gatilho sem `ready_for_review`, filtro de caminho no `pull_request`) |
| `trigger-and-secret-surface` | `pull_request_target`, `workflow_run` ou um passo lendo qualquer segredo além do `GITHUB_TOKEN` da execução |
| `job-budget` | job sem `timeout-minutes`, ou com um teto acima do orçamento do pipeline |

O `gates-wired` carrega uma tabela de 21 gates, lida de `.local/phases/19-production-operations.md` e de `make verify`. Uma tabela, e não uma lista de alvos: um gate que sai da tabela some do CI em silêncio, e um gate que sai de `verify` mas continua na tabela é exatamente o que a regra pega.

O scan da imagem é o único gate que a tabela aceita por uma forma alternativa — `make image-scan` exige um scanner instalado fora do repositório, e no CI o scanner vem por ação. A alternativa só conta quando ela **pergunta a mesma coisa**: a ação precisa examinar a imagem que aquela execução constrói (`${{ env.IMAGE }}`) e usar os parâmetros que o próprio alvo declara (`--severity CRITICAL,HIGH --ignore-unfixed --exit-code 1`, lidos da receita do `Makefile`). Um scan mais fraco que o gate do operador é recusado como scan mais fraco.

**A prova:** 22 casos de mutação, um por mutação que quebra exatamente uma regra, com a mutação aplicada por uma substituição que **falha se o texto alvo não existir** (um caso não pode passar sem mutar nada), mais os arquivos entregues auditados como estão. Além disso, 14 falsificações pelo fio sobre os workflows commitados — tag no lugar do SHA, `|| true` num gate, `severity` reduzida, `ignore-unfixed` desligado, scan de outra imagem, `ready_for_review` removido, `make test-e2e` trocado por um `echo`, porta do banco que o serviço não publica, job sem timeout, `contents: write`, `pull_request_target`, job sem a condição de rascunho, `govulncheck` no lugar de `make vuln` e um passo lido de outro segredo — cada uma recusada pela regra pretendida e restaurada por comparação.

Ele nunca reescreve nada: a correção pertence ao commit que mudou o workflow.

## 3. Rascunho é adiado, não aprovado

Os dois workflows têm `on.pull_request.types: [opened, synchronize, reopened, ready_for_review]` e **todos** os jobs carregam `if: github.event_name != 'pull_request' || github.event.pull_request.draft == false`.

As duas metades são necessárias e o auditor exige as duas: sem o gatilho, marcar o PR como pronto não iniciaria a verificação; sem a condição, cada push num rascunho rodaria a suíte inteira.

O que isso significa, e o que não significa:

- durante uma fase, o ciclo de microtarefas roda local (`make fmt-check`, os testes diretamente relacionados e os gates especializados: `.local/GIT_FLOW.md` §5);
- o gate da fase roda inteiro sobre o PR marcado como pronto, e o `finish` só mergeia com o CI completo verde;
- **nada é dispensado**: os mesmos dez jobs, os mesmos gates. Um rascunho não é uma verificação parcial aprovada; é uma verificação que ainda não começou.

Por isso `draft-skip-without-reduction` também recusa um filtro de caminho no `pull_request`: um filtro é uma segunda maneira de um merge passar sem a verificação completa, e ela não deixa rastro no PR.

## 4. Orçamento de tempo

`.local/git-flow.sh` espera 1800 segundos pelos checks do PR. Cada job declara `timeout-minutes`, e o auditor recusa acima de 30: um job que pode viver mais que a espera transforma um runner travado numa espera que expira, em vez de uma falha relatada. Os limites entregues são 30 minutos para os sete jobs de `verify` e 15 para os três de `supply-chain`.

Um job que estoura o próprio teto falha, e um check vermelho não tem merge (§5 de `GIT_FLOW.md`).

## 5. Ações de terceiro

Quatro ações, todas presas por commit com a tag ao lado: `actions/checkout`, `actions/setup-go`, `actions/setup-node`, `actions/dependency-review-action`, `aquasecurity/trivy-action` e `gitleaks/gitleaks-action`. Uma tag pode ser movida sob um workflow que já revisou aquele código; um SHA, não.

Subir uma versão é um commit que troca o SHA **e** o comentário, do mesmo jeito que subir uma base da imagem troca tag e digest (`DEPLOYMENT.md` §5). A permissão do token é `contents: read` — mais `pull-requests: read` no único job que comenta revisão de dependências —, e nenhum passo lê outro segredo que não o `GITHUB_TOKEN` que o GitHub emite para a execução.

## 6. O que ficou de fora, e por quê

- **`make test-load-smoke`** — mede capacidade contra uma instância preparada com dados sintéticos e k6, não correção. Não é gate de release: ele não está em `make verify` nem em nenhum job. Rodar é `K6_BASE_URL=… make test-load-smoke`.
- **Deploy** — o CI não promove nada. Promover é `deploy/deploy.sh` (`DEPLOYMENT.md` §5), executado por um operador, e continua desativado como automação.
- **O coletor do listener administrativo** — `/metrics` responde em loopback dentro de um contêiner distroless, então nenhum job o lê de fora. É a pendência registrada na P19-T06 e o que falta para o alerta de 5xx de [RUNBOOKS.md](RUNBOOKS.md) ter uma fonte fora do host.
- **`lint`** — `make lint` ainda é um alvo pendente: `make verify` o lista como não criado, e nenhum job o chama. [STACK.md](STACK.md) §6 prevê `golangci-lint` como ferramenta de desenvolvimento, mas o alvo não existe no `Makefile` e por isso ele não entra na tabela de gates: a tabela lista o que a fase exige, e um gate que exige uma configuração ainda não escrita seria uma linha decorativa. Quando o alvo existir, ele entra no `Makefile`, num job e na tabela — sem os três, `make audit-ci` recusa.

## 7. Reproduzir localmente

Cada job é uma linha de shell, então o pipeline é reproduzível na máquina:

- Go, Node e os `make` de `foundation`: instalar `sqlc@v1.29.0` e rodar `make verify`, com `ARENA_DATABASE_URL` apontando para um PostgreSQL alcançável;
- as jornadas de browser: `npm ci --prefix tools/e2e`, `npx --prefix tools/e2e playwright install-deps chromium` e `make test-e2e`;
- os gates que exigem daemon Docker: `make image-verify`, `make caddy-verify`, `make compose-verify`, `make backup-verify`, `make deploy-verify` (todos instalam o que precisam de Go);
- os scans: `make vuln` exige `govulncheck@v1.8.0`, `make image-scan` exige `trivy`, e ambos falham com mensagem explícita quando a ferramenta não está instalada.

`make verify` é a mesma coisa que o job `foundation` roda: o CI não tem um caminho de verificação próprio.

## 8. Regra de manutenção

- Um gate novo entra no `Makefile`, num job e na tabela do `tools/ciaudit`. Sem os três, `make audit-ci` recusa — e é isso que impede um gate de existir no documento e não no pipeline. A tabela carrega a superfície que a fase 19 exigiu; um gate de fase posterior que roda dentro de `make verify` — como `make audit-req` ([REQUIREMENTS.md](REQUIREMENTS.md) §8, P20) — aparece no alvo e nos mesmos jobs, e o `make audit-ci` é quem prova que a tabela não regrediu.
- O CI não pode ser mais fraco que o alvo que ele chama: parâmetros de scan, versão de scanner e conjunto de gates são lidos do `Makefile`, nunca redigitados.
- Achado de gate que precise de correção entra por um commit novo na branch da fase: nada de `--force`, de `--admin`, de `--no-verify` ou de limiar reduzido para obter verde.
