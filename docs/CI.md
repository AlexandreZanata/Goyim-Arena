# Verificação rápida de integração e certificação de release

**Status:** cadência atualizada em 2026-09-24. O CI verifica; ele **não** publica, não faz deploy e não tem credencial de produção.

## Política vigente

- Todo PR (inclusive rascunho) e push em `main` executa `quick.yml` / `Quick verification`: formato Go, compilação de todos os pacotes e testes Go (`go test -run '^$' ./...`), testes reais dos domínios wallet/identity/arguments/arenas e do auditor CI, e TypeScript estrito. O check é obrigatório na proteção da `main`; não substitui testes direcionados de integração/segurança.
- Cada microtarefa executa localmente testes de comportamento direcionados e sua validação mínima, incluindo PostgreSQL real, falhas, autorização e concorrência em Q0 quando aplicável. A fase só fecha após seus gates especializados, `make quick-verify` e o check remoto verde. A `main` pode conter fases ainda não certificadas para release.
- `verify.yml` e `supply-chain.yml` rodam por `workflow_dispatch` no candidato de versão e em tag `v*`, não por push comum nem PR. P30 e P44 são marcos de certificação completa. Tag estável, release e deploy requerem todos os jobs completos verdes no mesmo SHA; falha exige novo commit/candidato, nunca mover uma tag publicada.
- A troca economiza repetição, mas aumenta o tempo até detectar uma regressão fora dos testes direcionados. Não alegue qualidade certificada antes da matriz completa. O auditor `tools/ciaudit` verifica os gatilhos e a presença do check rápido.

Este documento é a superfície do pipeline: quais gates um merge exige, em que job cada um roda, o que o `tools/ciaudit` recusa, quanto tempo a verificação pode levar e o que ficou deliberadamente de fora. Onde ele afirma um número, o número foi medido no commit que o escreveu.

## 1. O que uma versão exige (não cada merge)

Os dois workflows completos, executados apenas no versionamento, contêm os jobs abaixo. A revisão de dependências por ação de PR foi retirada da esteira de release porque precisa de contexto de PR; os scans de dependências e imagem continuam em `source-scans`/`image-scan`.

| Workflow | Job | Gates | Ferramentas que o job instala |
| --- | --- | --- | --- |
| `verify` | `foundation` | `make verify` — formatação, drift dos artefatos gerados, unit, integração PostgreSQL, race selecionado, migrations, contrato OpenAPI, segurança, build do frontend, `tsc` estrito, medição do frontend, auditoria de i18n, auditoria de rastreabilidade dos requisitos, auditoria do próprio CI, auditoria do catálogo de regras, auditoria dos waivers, auditoria do handoff | Go, Node, sqlc, PostgreSQL 18.4 (serviço) |
| `verify` | `browser` | `make test-e2e` — jornadas críticas em Chromium | Go, Node, Playwright (pinado em `tools/e2e`), PostgreSQL 18.4 (serviço) |
| `verify` | `image` | `make image-verify` e o scan da imagem construída | Go, Docker, trivy (ação) |
| `verify` | `ingress` | `make caddy-verify` | Go, Docker, openssl |
| `verify` | `stack` | `make compose-verify` | Go, Docker, openssl |
| `verify` | `backup` | `make backup-verify` | Go, Docker, openssl |
| `verify` | `deploy` | `make deploy-verify` | Go, Docker, openssl |
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
| `release-only-cadence` | workflow quick ausente/filtrado/condicional, ou suíte completa voltando a rodar em todo PR/push de `main`, ou sem gatilho de tag |
| `trigger-and-secret-surface` | `pull_request_target`, `workflow_run` ou um passo lendo qualquer segredo além do `GITHUB_TOKEN` da execução |
| `job-budget` | job sem `timeout-minutes`, ou com um teto acima do orçamento do pipeline |

O `gates-wired` carrega uma tabela de 21 gates, lida de `.local/phases/19-production-operations.md` e de `make verify`. Uma tabela, e não uma lista de alvos: um gate que sai da tabela some do CI em silêncio, e um gate que sai de `verify` mas continua na tabela é exatamente o que a regra pega.

O scan da imagem é o único gate que a tabela aceita por uma forma alternativa — `make image-scan` exige um scanner instalado fora do repositório, e no CI o scanner vem por ação. A alternativa só conta quando ela **pergunta a mesma coisa**: a ação precisa examinar a imagem que aquela execução constrói (`${{ env.IMAGE }}`) e usar os parâmetros que o próprio alvo declara (`--severity CRITICAL,HIGH --ignore-unfixed --exit-code 1`, lidos da receita do `Makefile`). Um scan mais fraco que o gate do operador é recusado como scan mais fraco.

**A prova:** 22 casos de mutação, um por mutação que quebra exatamente uma regra, com a mutação aplicada por uma substituição que **falha se o texto alvo não existir** (um caso não pode passar sem mutar nada), mais os arquivos entregues auditados como estão. Além disso, 14 falsificações pelo fio sobre os workflows commitados — tag no lugar do SHA, `|| true` num gate, `severity` reduzida, `ignore-unfixed` desligado, scan de outra imagem, `ready_for_review` removido, `make test-e2e` trocado por um `echo`, porta do banco que o serviço não publica, job sem timeout, `contents: write`, `pull_request_target`, job sem a condição de rascunho, `govulncheck` no lugar de `make vuln` e um passo lido de outro segredo — cada uma recusada pela regra pretendida e restaurada por comparação.

Ele nunca reescreve nada: a correção pertence ao commit que mudou o workflow.

## 3. PR rápido não é certificação

`quick.yml` roda também em rascunhos. A suíte completa não roda automaticamente no PR: é uma verificação adiada para o candidato de versão, não uma aprovação implícita do código. O auditor recusa filtros de caminho ou condição no job rápido e exige os gatilhos de versão nos workflows completos. O `finish` só mergeia depois do gate local rápido e do check remoto verde, além dos testes direcionados que o executor registrou.

## 4. Orçamento de tempo

`.local/git-flow.sh` espera 1800 segundos pelos checks do PR, mas o único check de integração obrigatório é `Quick verification` (12 minutos de teto). Os jobs de versão mantêm tetos de 30 minutos por job, executados fora do fluxo de cada fase.

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
- os gates que exigem daemon Docker: `make image-verify`, `make caddy-verify`, `make compose-verify`, `make backup-verify`, `make deploy-verify`, `make migration-audit`, `make disaster-drill` (todos instalam o que precisam de Go); o `make testenv-verify` (P22-T01) exige ainda uma build do frontend, porque sobe a aplicação de verdade e dirige a prontidão dela pela porta do ambiente; o `disaster-drill` exige ainda `k6`, um navegador instalado pelo runner do `tools/e2e` e uma build do frontend, como `test-e2e` e `test-load-smoke`; a `make release-verify` (P20-T07) exige o daemon, um PostgreSQL alcançável em `127.0.0.1:54329`, Node com npm e uma árvore limpa, porque ela **cria um checkout limpo do commit** e roda o próprio `make verify` duas vezes;
- os scans: `make vuln` exige `govulncheck@v1.8.0`, `make image-scan` exige `trivy`, e ambos falham com mensagem explícita quando a ferramenta não está instalada. O `govulncheck` tem de ser construído com o Go que o `go.mod` declara (`GOTOOLCHAIN=go1.27.1 go install golang.org/x/vuln/cmd/govulncheck@v1.8.0`): um binário construído com uma versão anterior não processa os pacotes e o gate fica vermelho por ambiente, não por vulnerabilidade;
- o ciclo de vida dos testes, em uma linha: `make test-isolation` (P22-T06), que exige um PostgreSQL alcançável (o mesmo de `make test-unit`) e roda a suíte inteira com ordem embaralhada e `-parallel=16`, depois de exigir que a fixture de vazamento proposital (`internal/platform/testguard/testdata/leak/`) faça uma execução vermelha nomeando cada regra, e comparando a máquina antes e depois — banco descartável, conexão e diretório temporário — com `tools/isolationaudit`. Ele fica fora de `make verify` porque roda a suíte inteira, como `testenv-verify`;
- a execução offline e reprodutível, em uma linha: `make test-offline` (P22-T08), que escreve o manifesto de ferramentas, imagens e lockfiles duas vezes e exige bytes iguais — o manifesto é função da árvore, sem instante, host ou caminho absoluto, e o leitor recusa a ferramenta que diverge do pino, a imagem que constrói ou publica sem digest, a imagem que o `compose.yaml` roda sem digest registrado e o `package.json` sem lockfile ao lado. Depois ele instala dos lockfiles aprovados (`go mod download`, `npm ci` duas vezes) e **repete a metade que lê** com egress negado (`go mod verify` e o `npm ci --dry-run` da árvore travada), prova que um processo que tenta alcançar o registro é recusado por regra nomeando o host e não arquiva nada, e roda `make test-unit` e `make test-integration` offline, verdes, com **zero** tentativas, arquivando o manifesto e a declaração de evidência de cada um — que o formato da P22-T07 aceita, e recusa quando a declaração é adulterada. Exige um PostgreSQL alcançável (o mesmo de `make test-unit`) e Node com os dois lockfiles, e por isso fica fora de `make verify`;
- a auditoria de segurança completa, em uma linha: `make security-audit` (P20-T04), que roda as doze áreas e julga o registro [SECURITY_AUDIT.md](SECURITY_AUDIT.md) contra o modelo de ameaças;
- a auditoria final de internacionalização, em uma linha: `make i18n-audit` (P20-T09), que executa as áreas que a fase nomeia — cobertura de catálogo, pseudo-locale, snapshots, emails, Problem Details, SEO, moeda, plural, timezone, cache e texto hardcoded — e julga o registro [I18N_AUDIT.md](I18N_AUDIT.md) contra a árvore medida agora;
- a revisão de privacidade e moderação, em uma linha: `make privacy-audit` (P20-T06), que roda as sete áreas de ciclo de vida de dados e julga o registro [PRIVACY_AUDIT.md](PRIVACY_AUDIT.md) contra o código que ele descreve;
- a verificação final reproduzível, em uma linha: `make release-verify` (P20-T07), que cria um checkout limpo do commit, instala as dependências só pelos lockfiles, sobe o PostgreSQL, roda `make verify` duas vezes, constrói a imagem e a exercita com o smoke, e escreve [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md);
- o handoff do backend, em duas linhas: `make handoff-check` (P20-T08), que julga o [README](../README.md) contra a árvore e roda dentro de `make verify`, e `make handoff-walkthrough`, que segue num checkout limpo os blocos que a própria página declara — a jornada de comandos que ela manda colar e o servidor que ela manda subir — e sonda as superfícies que ela nomeia, com o daemon Docker disponível e a porta `127.0.0.1:8080` livre.

`make verify` é a mesma coisa que o job `foundation` roda: o CI não tem um caminho de verificação próprio.

`make security-audit` (P20-T04) é uma exceção declarada do mesmo tipo: ele lê o registro de [SECURITY_AUDIT.md](SECURITY_AUDIT.md), resolve cada evidência citada, confere o registro contra o modelo de ameaças e roda as **doze** execuções da auditoria — entre elas `make vuln` e duas suítes com `-race`. Ele **não** está ligado a nenhum job, e não precisa estar: os dois gates que a fase exige dele já estão na esteira (`make test-security` no `foundation`, `make vuln` no `source-scans`), e o portão é a soma deles com as outras dez áreas, que é trabalho de release e não de merge. É por isso que ele também não entra em `make verify` — um gate de integração não é um release, como o `release-gate` das decisões humanas. A evidência que ele produz é [SECURITY_AUDIT.md](SECURITY_AUDIT.md).

`make release-verify` (P20-T07) é a exceção mais cara da lista, e é a que fecha a fase: ela **roda o próprio `make verify` duas vezes** e por isso não pode estar dentro dele. Ela exige daemon Docker, um PostgreSQL alcançável na porta que os harnesses procuram (127.0.0.1:54329, a mesma que o CI declara como serviço) e uma árvore limpa. O que ela produz é [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md), e o que a torna verificável é ela recusar sozinha: um comando que rodou uma vez, uma execução vermelha, um lockfile ausente ou contornado, uma imagem que ninguém construiu ou exercitou, uma limitação ausente e um `git fsck` com erro são violações nomeadas, e um vermelho **aborta antes de escrever o documento** — uma verificação que falha nunca deixa para trás algo que se lê como aprovado.

`make handoff-check` (P20-T08) é a exceção ao contrário, e é a única desta lista que **entrou** em `make verify`: ele lê o [README](../README.md) e recusa toda afirmação que a árvore contradiz — um comando que não existe no `Makefile`, uma variável que não está no `.env.example`, um caminho que não está lá, um subcomando que o binário não tem — mais as declarações que a caminhada precisa (os marcadores do bloco que a página manda colar, os passos dele e as superfícies que ela nomeia). Ele é barato, não exige ambiente nenhum e é gate de merge de verdade: renomear um alvo sem atualizar a página que o manda rodar é exatamente o que ele existe para recusar. A outra metade, `make handoff-walkthrough`, **não** entra: segue os blocos que o próprio README declara num checkout limpo do commit, roda os comandos que a página manda colar e sobe o servidor como ela manda para sondar as superfícies que ela nomeia — exige daemon Docker, um checkout limpo e a porta que o documento usa livre.

`make i18n-audit` (P20-T09) é a exceção de outra ordem: ele **entrou** em `make verify`, e a única área que ele não executa é a das jornadas de navegador. As outras onze são os próprios portões entregues (`make generate-check`, `go test -tags pseudolocale ./internal/i18n/...`, os testes do renderer, das notificações, do `httperror` e do adapter HTML das Arenas, `make test-web` e `make audit-i18n`), então executá-los de novo é quase de graça — o cache de teste do Go responde pelas repetições — e o que a repetição compra é que "a área foi executada" seja decidido ali e não lembrado num documento. Depois ele julga o registro: as contagens do catálogo, os placeholders, as chaves inalcançáveis, os literais com forma de chave, os snapshots de email, o vocabulário de códigos, os plurais, o `hreflang`, o uso proibido de `toLocaleString`, a dimensão de locale do cache, o mecanismo do pseudo-locale, os idiomas das jornadas e a revisão humana — cada afirmação recalculada a partir da árvore, nunca lida do documento. As jornadas exigem Chromium e um PostgreSQL descartável, que `tools/e2e/harness.sh` compõe e o CI roda em `make test-e2e`; o que este portão prova sobre elas é estrutural (as suítes existem, iteram pelos dois idiomas e estão ligadas a um alvo do `Makefile`), e o registro guarda a execução com a data.

`make privacy-audit` (P20-T06) é uma exceção declarada do mesmo tipo: ele lê o registro de [PRIVACY_AUDIT.md](PRIVACY_AUDIT.md) e roda as **sete** execuções da revisão — as suítes de exportação pública e pessoal, exclusão, retenção, payload de analytics, logs e moderação. Ele **não** está ligado a nenhum job, e não precisa estar: as sete execuções já são partes de `make test-integration`/`test-unit`/`test-race`, e o que a fase exige do portão é o **julgamento** — o registro contra o código, nos dois sentidos —, que é trabalho de release e não de merge. É por isso que ele também não entra em `make verify`. A evidência que ele produz é [PRIVACY_AUDIT.md](PRIVACY_AUDIT.md).

`make migration-audit` (P20-T03) é uma exceção declarada: sobe um PostgreSQL 18.4 descartável fixado por digest, exercita o ciclo de vida das migrations com o runner que a aplicação usa e recusa com base no que mediu. Ele **não** está ligado a nenhum job; hoje é um gate que o operador roda, e a ligação a um job — com o ajuste correspondente em `tools/ciaudit`, que é quem exige que todo gate do plano esteja em algum job — pertence à verificação reproduzível da fase. A evidência que ele produz é [MIGRATION_AUDIT.md](MIGRATION_AUDIT.md).

`make disaster-drill` (P20-T05) é uma exceção declarada do mesmo tipo: restaura um backup num ambiente isolado, sobe a aplicação nos dados que voltaram, compara o ledger, mede RPO e RTO, roda o baseline de carga e exercita os dois provedores indisponíveis. Ele **não** está ligado a nenhum job e não está em `make verify`: exige daemon Docker, `k6`, navegador e uma build do frontend, e um gate de merge não é um release. O que o torna verificável é o seu próprio portão: o exercício escreve os fatos que mediu e `drillaudit check` recusa um número fora do teto declarado, um limiar não registrado, um provedor não exercitado nos dois sentidos ou um ledger que não voltou igual — inclusive na suíte de testes, que julga o documento entregue como ele está. A evidência que ele produz é [DISASTER_DRILL.md](DISASTER_DRILL.md).

## 8. Regra de manutenção

- Um gate completo novo entra no `Makefile`, num job de versão e na tabela `requiredGates` do `tools/ciaudit`. O gate rápido é exigido separadamente por `release-only-cadence`, inclusive sua presença, gatilhos, ausência de filtro e invocação de `make quick-verify`. Gates posteriores que entram em `make verify` são alcançados pelo job de versão sem rodar em todo PR.
- O CI não pode ser mais fraco que o alvo que ele chama: parâmetros de scan, versão de scanner e conjunto de gates são lidos do `Makefile`, nunca redigitados.
- Achado de gate que precise de correção entra por um commit novo na branch da fase: nada de `--force`, de `--admin`, de `--no-verify` ou de limiar reduzido para obter verde.
