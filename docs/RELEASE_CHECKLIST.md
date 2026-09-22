# Checklist de release do backend (P20-T07)

Verificação reproduzível de ponta a ponta, rodada em uma árvore limpa no
commit `ca19777992242ba223ff0014ca68caa52efb0120`, com cada comando da fase executado **duas vezes**. Este documento é
gerado por essa execução (`tools/releaseverify/verify.sh`): a prosa descreve o que
foi medido e o bloco no fim carrega os números, para que a frase e a medição não
possam divergir. Para rodar de novo: `make release-verify`.

## 1. Veredito

**A verificação técnica passa; a release não está autorizada.** As duas coisas são
independentes e o documento registra as duas: nada nesta execução falhou (todo
comando passou em duas execuções, a árvore terminou limpa e `git fsck` não
reportou erro), e ainda assim `make release-gate` recusa, porque há decisão do
titular em aberto — e uma decisão não é um defeito que o código possa corrigir.

Em aberto, nomeadas pelo próprio portão:

- terms-of-use
- security-channel

Decididas, com o passo de execução ainda pendente (o portão os lista e não são
motivo de recusa):

- repository-license
- data-subject-channel
- retention-policy
- launch-markets

## 2. Onde e com o quê

- **Árvore:** git worktree limpo no commit, criado por tools/releaseverify/verify.sh e descartado no fim.
- **Commit:** `ca19777992242ba223ff0014ca68caa52efb0120` na branch `phase-20-release-readiness`. O documento entra no commit seguinte: a árvore
  verificada é a desse commit, e o único arquivo que muda depois dela é este.
- **Máquina:** Linux 7.1.5-76070105-generic/x86_64, 16 processador(es).
- **Toolchain:** go1.27.1, Node v24.15.0, npm 11.14.1, Docker 29.1.3/29.1.3, sqlc v1.29.0.
- **PostgreSQL:** 18.4 (Debian 18.4-1.pgdg13+1).
- **Origem do banco:** Os harnesses de integração procuram o PostgreSQL em 127.0.0.1:54329 — a porta que o compose.yaml publica e que o CI declara como serviço —, e este run usou o PostgreSQL que já respondia na porta.
- **Repositório:** 1082 arquivo(s) rastreado(s); `git fsck` ok com 0 erro(s).
- **Diretório local:** 0 arquivo(s) rastreado(s) (tem de ser zero: o plano local nunca
  vai para o Git).

## 3. Dependências: somente lockfiles

Nada é instalado por versão flutuante. Cada dependência vem do lockfile que o commit
versiona, e o portão recusa o documento em que algum comando instale fora de um:

| Lockfile | Digest (sha256) | Instalado por |
|---|---|---|
| `go.sum` | `57a051c0abe2b6b5…` | `go mod download` |
| `web/package-lock.json` | `63d91f35e3565ade…` | `npm ci --prefix web` |
| `tools/e2e/package-lock.json` | `dbc593221c116a3a…` | `npm ci --prefix tools/e2e` |

## 4. Os comandos, duas vezes cada

| Comando | Execuções | Tempo |
|---|---|---|
| `GOBIN='/tmp/arena-release-verify-66ZPd8/bin' go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0` | 2 | 5.9s |
| `go mod download` | 2 | 0.0s |
| `npm ci --prefix web` | 2 | 1.1s |
| `npm ci --prefix tools/e2e` | 2 | 0.6s |
| `docker run --rm --network host --env PGPASSWORD --env PGHOST=127.0.0.1 --env PGPORT=54329 --env PGUSER=arena --env PGDATABASE=arena postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636 psql --tuples-only --no-align --command 'SHOW server_version'` | 2 | 0.4s |
| `ARENA_DATABASE_URL='postgres://arena:arena-local-dev@127.0.0.1:54329/arena?sslmode=disable' make verify` | 2 | 143.8s |
| `IMAGE='goyim-arena:release-verify' make image-build` | 2 | 28.8s |
| `ARENA_POSTGRES_IMAGE='postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636' IMAGE='goyim-arena:release-verify' make image-verify` | 2 | 9.1s |
| `git fsck --no-progress` | 2 | 0.4s |

Tempo somado dos comandos: **190.1s**. Cada linha é o que a fase exige: o mesmo
comando duas vezes, com o mesmo resultado.

## 5. A imagem e o smoke

- **Imagem:** `goyim-arena:release-verify` (`sha256:1f561aec4f96cfedeae9ca8ab3823fe2b38381fb88216777a32eed4541318944`, 7.1 MiB).
- **Smoke:** `make image-verify` — a imagem sobe com filesystem somente leitura contra um PostgreSQL descartável, aplica as próprias migrations, serve uma página e o asset com hash que ela referencia.

## 6. Limitações reais

O que esta verificação **não** estabelece, declarado em vez de omitido:

- Limitações do make verify: o alvo cobre o que não exige ambiente próprio, e os gates que exigem k6, navegador, trivy ou daemon Docker (test-e2e, test-load-smoke, image-scan, caddy-verify, compose-verify, backup-verify, deploy-verify, migration-audit, disaster-drill e vuln) não rodaram aqui — cada um tem o próprio alvo e a própria evidência versionada.
- A verificação mede a máquina desta execução (Linux 7.1.5-76070105-generic/x86_64, 16 processador(es), Node v24.15.0, Docker 29.1.3/29.1.3): dois commits verificados em máquinas diferentes são dois fatos diferentes, e este documento registra qual máquina mediu.
- A imagem não foi publicada num registry, então o digest registrado é o da configuração local (docker image inspect .Id), não um digest de manifesto de registry.
- O portão de release continua vermelho enquanto a decisão do titular estiver em aberto: o commit verificado não é uma release autorizada, e nenhuma verificação técnica muda isso.
- O dataset dos testes de integração é criado pelos próprios harnesses: esta verificação mede correção, não volume, latência nem capacidade — isso é tests/load e o exercício da P20-T05.
- O documento entra no commit seguinte ao verificado: a árvore verificada é a do commit registrado, e o único arquivo que muda depois dela é este.

## 7. Depois daqui

- Fechar as duas decisões humanas do titular e rodar ./.local/git-flow.sh finish, que marca o PR #62 como pronto e só mergeia com o CI verify verde.
- P20-T08 (handoff) e P20-T09 (auditoria final de i18n) fecham a fase; este checklist é re-rodado depois do último commit da fase.

```json
{
  "version": 1,
  "verified_on": "2026-09-22",
  "commit": "ca19777992242ba223ff0014ca68caa52efb0120",
  "branch": "phase-20-release-readiness",
  "checkout": {
    "kind": "git worktree limpo no commit, criado por tools/releaseverify/verify.sh e descartado no fim",
    "clean": true,
    "local_tracked_files": 0,
    "generated": "docs/RELEASE_CHECKLIST.md",
    "dirty": [
      "docs/RELEASE_CHECKLIST.md"
    ],
    "overlay": []
  },
  "host": {
    "os": "Linux 7.1.5-76070105-generic",
    "arch": "x86_64",
    "cpus": 16,
    "go": "go1.27.1",
    "node": "v24.15.0",
    "npm": "11.14.1",
    "docker": "29.1.3/29.1.3",
    "sqlc": "v1.29.0",
    "postgres": "18.4 (Debian 18.4-1.pgdg13+1)"
  },
  "lockfiles": [
    {
      "path": "go.sum",
      "sha256": "57a051c0abe2b6b56f18d2c3c6c67b255946fd8f8d5568accc6d54914ab4e267",
      "installs": "go mod download",
      "note": "Dependência instalada a partir deste lockfile, nunca por versão flutuante."
    },
    {
      "path": "web/package-lock.json",
      "sha256": "63d91f35e3565ade603c8159dc7f58afd4b40981d138ef47adea2b314f227355",
      "installs": "npm ci --prefix web",
      "note": "Dependência instalada a partir deste lockfile, nunca por versão flutuante."
    },
    {
      "path": "tools/e2e/package-lock.json",
      "sha256": "dbc593221c116a3acc38b19290c07f49c2265d9ce7eb9033fb884049cff30ffd",
      "installs": "npm ci --prefix tools/e2e",
      "note": "Dependência instalada a partir deste lockfile, nunca por versão flutuante."
    }
  ],
  "commands": [
    {
      "key": "sqlc-toolchain",
      "command": "GOBIN='/tmp/arena-release-verify-66ZPd8/bin' go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0",
      "runs": [
        {
          "seconds": 5.3,
          "exit_code": 0
        },
        {
          "seconds": 0.6,
          "exit_code": 0
        }
      ],
      "note": "Instala o sqlc na versão que o repositório fixa, fora do PATH do operador."
    },
    {
      "key": "go-modules",
      "command": "go mod download",
      "runs": [
        {
          "seconds": 0,
          "exit_code": 0
        },
        {
          "seconds": 0,
          "exit_code": 0
        }
      ],
      "note": "Baixa os módulos exatamente nas versões que go.sum fixa."
    },
    {
      "key": "web-modules",
      "command": "npm ci --prefix web",
      "runs": [
        {
          "seconds": 0.8,
          "exit_code": 0
        },
        {
          "seconds": 0.3,
          "exit_code": 0
        }
      ],
      "note": "Instala o frontend a partir de web/package-lock.json."
    },
    {
      "key": "e2e-modules",
      "command": "npm ci --prefix tools/e2e",
      "runs": [
        {
          "seconds": 0.3,
          "exit_code": 0
        },
        {
          "seconds": 0.3,
          "exit_code": 0
        }
      ],
      "note": "Instala as jornadas de navegador a partir de tools/e2e/package-lock.json."
    },
    {
      "key": "database",
      "command": "docker run --rm --network host --env PGPASSWORD --env PGHOST=127.0.0.1 --env PGPORT=54329 --env PGUSER=arena --env PGDATABASE=arena postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636 psql --tuples-only --no-align --command 'SHOW server_version'",
      "runs": [
        {
          "seconds": 0.2,
          "exit_code": 0
        },
        {
          "seconds": 0.2,
          "exit_code": 0
        }
      ],
      "note": "Os harnesses de integração procuram o PostgreSQL em 127.0.0.1:54329 — a porta que o compose.yaml publica e que o CI declara como serviço —, e este run usou o PostgreSQL que já respondia na porta."
    },
    {
      "key": "verify",
      "command": "ARENA_DATABASE_URL='postgres://arena:arena-local-dev@127.0.0.1:54329/arena?sslmode=disable' make verify",
      "runs": [
        {
          "seconds": 108.4,
          "exit_code": 0
        },
        {
          "seconds": 35.4,
          "exit_code": 0
        }
      ],
      "note": "O gate de integração completo, o mesmo que o CI roda no job foundation."
    },
    {
      "key": "image-build",
      "command": "IMAGE='goyim-arena:release-verify' make image-build",
      "runs": [
        {
          "seconds": 28.4,
          "exit_code": 0
        },
        {
          "seconds": 0.4,
          "exit_code": 0
        }
      ],
      "note": "Constrói a imagem de produção a partir do Dockerfile."
    },
    {
      "key": "image-smoke",
      "command": "ARENA_POSTGRES_IMAGE='postgres:18.4@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636' IMAGE='goyim-arena:release-verify' make image-verify",
      "runs": [
        {
          "seconds": 4.5,
          "exit_code": 0
        },
        {
          "seconds": 4.6,
          "exit_code": 0
        }
      ],
      "note": "Sobe a imagem com filesystem somente leitura contra um PostgreSQL descartável, prova que ela aplica as próprias migrations, serve uma página e o asset com hash que ela referencia."
    },
    {
      "key": "fsck",
      "command": "git fsck --no-progress",
      "runs": [
        {
          "seconds": 0.2,
          "exit_code": 0
        },
        {
          "seconds": 0.2,
          "exit_code": 0
        }
      ],
      "note": "git fsck na árvore limpa."
    }
  ],
  "image": {
    "reference": "goyim-arena:release-verify",
    "id": "sha256:1f561aec4f96cfedeae9ca8ab3823fe2b38381fb88216777a32eed4541318944",
    "digest": "sha256:1f561aec4f96cfedeae9ca8ab3823fe2b38381fb88216777a32eed4541318944",
    "size_bytes": 7483336,
    "smoke": "make image-verify",
    "smoke_detail": "a imagem sobe com filesystem somente leitura contra um PostgreSQL descartável, aplica as próprias migrations, serve uma página e o asset com hash que ela referencia",
    "smoke_ok": true
  },
  "repository": {
    "tracked_files": 1082,
    "fsck": "ok",
    "fsck_errors": 0,
    "main_behind_origin": false
  },
  "governance": {
    "command": "make release-gate",
    "blocked": true,
    "open": [
      "terms-of-use",
      "security-channel"
    ],
    "pending": [
      "repository-license",
      "data-subject-channel",
      "retention-policy",
      "launch-markets"
    ],
    "note": "O portão das decisões humanas do lançamento (make release-gate): vermelho aqui não é defeito de código, é decisão em aberto."
  },
  "limits": [
    "Limitações do make verify: o alvo cobre o que não exige ambiente próprio, e os gates que exigem k6, navegador, trivy ou daemon Docker (test-e2e, test-load-smoke, image-scan, caddy-verify, compose-verify, backup-verify, deploy-verify, migration-audit, disaster-drill e vuln) não rodaram aqui — cada um tem o próprio alvo e a própria evidência versionada.",
    "A verificação mede a máquina desta execução (Linux 7.1.5-76070105-generic/x86_64, 16 processador(es), Node v24.15.0, Docker 29.1.3/29.1.3): dois commits verificados em máquinas diferentes são dois fatos diferentes, e este documento registra qual máquina mediu.",
    "A imagem não foi publicada num registry, então o digest registrado é o da configuração local (docker image inspect .Id), não um digest de manifesto de registry.",
    "O portão de release continua vermelho enquanto a decisão do titular estiver em aberto: o commit verificado não é uma release autorizada, e nenhuma verificação técnica muda isso.",
    "O dataset dos testes de integração é criado pelos próprios harnesses: esta verificação mede correção, não volume, latência nem capacidade — isso é tests/load e o exercício da P20-T05.",
    "O documento entra no commit seguinte ao verificado: a árvore verificada é a do commit registrado, e o único arquivo que muda depois dela é este."
  ],
  "next": [
    "Fechar as duas decisões humanas do titular e rodar ./.local/git-flow.sh finish, que marca o PR #62 como pronto e só mergeia com o CI verify verde.",
    "P20-T08 (handoff) e P20-T09 (auditoria final de i18n) fecham a fase; este checklist é re-rodado depois do último commit da fase."
  ]
}
```
