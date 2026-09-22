# Exercício de desastre e carga (P20-T05)

**Status:** evidência de um exercício, não uma garantia de capacidade. Os números abaixo foram medidos; o que não foi medido não está aqui.

**Executado em:** 2026-09-22 · **Commit:** `59397d3` · **Máquina:** Linux 7.1.5-76070105-generic, 16 CPU(s), Docker 29.1.3

## 1. O que o exercício fez

O caminho é o de um incidente, não um caminho paralelo: os mesmos scripts que a operação usa (`deploy/backup/base-backup.sh`, `deploy/backup/restore.sh`) sobre um PostgreSQL descartável, com o arquivamento contínuo que o `compose.production.yaml` declara. O dataset é sintético (seed `drill-168814`, 3 conta(s), 1000 INK creditado pelo próprio caso de uso da carteira).

1. o estado financeiro foi fotografado às **2026-09-22 17:02:53.856754** (linha de base);
2. o alvo escolhido foi **2026-09-22 17:02:54.916929**;
3. depois do alvo o exercício escreveu mais dados, que o restore **não** pode trazer: 0 linha(s) ficaram de fora por desenho;
4. o primário e o diretório de dados foram **destruídos** às **2026-09-22 17:02:56.551935** — o que sobrou foi o armazenamento;
5. a recuperação rodou `deploy/backup/restore.sh --base drill-2026-09-22T17-02-52Z --target-time 2026-09-22 17:02:54.916929`;
6. a aplicação subiu contra o cluster restaurado e respondeu perguntas reais sobre os dados que voltaram.

## 2. Integridade financeira

O que voltou é o mesmo dinheiro: **3 transação(ões) de ledger** em 3 operação(ões), 3 conta(s) e 3 carteira(s), com o ledger hasheando `5f70b120638d25a9597f83ea3ddb0a25` e a projeção `413c73014770c8e5b5e94c8a02ffc0f2` antes e depois do desastre. Nada foi criado, nada foi perdido: as duas leituras — a de antes e a de depois — são idênticas, e é isso que a comparação afirma linha a linha.

Saldo total antes: FREE_INK=3000, PURCHASED_INK=0. Depois: FREE_INK=3000, PURCHASED_INK=0.

A comparação não recusou nada: cada carteira voltou com o saldo que tinha, a projeção continua igual ao que o ledger deriva (a invariante da migration 00008), nenhum saldo ficou negativo e o agregado continua sendo a soma das partes.

## 3. RPO e RTO medidos

**RPO observado: 5s** (o commit mais novo que voltou é de 2026-09-22 17:02:51.884752, e o primário foi destruído depois dele) contra o limite de `archive_timeout` que o próprio servidor declara, **300s**. O atraso do arquivo no fim do exercício foi de 1s. A meta declarada da release é de 900s.

**RTO: 2s** da destruição até o servidor restaurado aceitar escrita, e **3s** até a aplicação responder na superfície pública sobre os dados restaurados. A meta declarada é de 14400s.

Os dois números são do exercício e não prometem nada: uma recuperação real paga rede, disco e decisão humana, e o que este exercício mede é o piso do processo.

## 4. Provedor indisponível

**Email.** O trabalho de entrega rodou com o provedor inalcançável: **2 tentativa(s)** registrada(s) com o código `JOB_HANDLER_ERROR`, o job **sobreviveu** na fila (uma mensagem perdida numa indisponibilidade é uma mensagem que o produto descartou em silêncio) e, quando o provedor voltou a responder, a mesma mensagem foi entregue em 99s.

**Stripe.** A superfície do provedor foi sondada e o veredito é `surface_absent`, com **0** linha(s) financeira(s) criada(s) enquanto o provedor estava inalcançável. Sondagens:

- POST `/api/v1/webhooks/stripe` → **404** (esperado 400)
- POST `/api/v1/me/billing/checkout` → **404** (esperado 401)
- GET `/api/v1/me/billing/subscription` → **404** (esperado 401)
- GET `/api/v1/me/wallet` → **404** (esperado 401)

Uma superfície que não está composta é um **achado**, e não uma aprovação: dono `bootstrap/rel`, trabalho seguinte `compose in 'arena server' the JSON surfaces the contract declares (the drill probed /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe) and register the Stripe webhook route; until then the payment boundary (and the wallet read of the restored data) is not reachable by the product`.

## 5. Baseline de carga

Workload versionado `tests/load/smoke.js` (dataset `drill-168814`), k6, 21s. Limites declarados pelo próprio script e o que o exercício mediu:

| Métrica | Limite declarado | Medido |
| --- | --- | --- |
| `http_req_failed` | `rate<0.05` | 0 |
| `http_req_duration{workload:cache-cold}` | `p(95)<1000` | 0.3097252499999999 |
| `http_req_duration{workload:cache-hot}` | `p(95)<300` | 0.31246949999999996 |
| `http_req_duration{workload:login}` | `p(95)<1500` | 0.31890589999999996 |
| `http_req_duration{workload:position}` | `p(95)<1000` | 0.29713484999999995 |
| `http_req_duration{workload:argument-wallet}` | `p(95)<1000` | 0.27988155 |
| `http_req_duration{workload:webhook-replay}` | `p(95)<1000` | 0.3343214999999999 |
| `http_req_duration{workload:arena-viral}` | `p(95)<500` | 0.28227085 |

**Nota:** as latências acima são as das respostas que a composição entregue dá aos caminhos do workload — /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe responderam 404 —, e não as de um caminho de pagamento ou de carteira montados: enquanto essas superfícies não estiverem compostas, o baseline mede o que existe, e o que existe não é o caminho que a fase pede para medir.

Nenhum limite foi cruzado: o baseline está registrado sem erro crítico.

## 6. Jornadas contra os dados restaurados

A aplicação foi subida contra o cluster **restaurado** e respondeu:

- GET `/health/ready` → **200**
- GET `/login` → **200**
- GET `/arenas/drill-arena-drill-168814` → **200**
- POST `/login` → **303**
- POST `/login` → **401**
- GET `/arenas/drill-arena-drill-168814` → **200**

E2E: executado e verde. the versioned harness drove its journeys green against the restored cluster (it provisions its own database inside it, because a journey's fixtures are not a customer's data: what the drill reads out of the restored data is the smoke probes and the ledger comparison above)

## 7. Limites registrados, não escondidos

- a superfície JSON que a composição entregue monta não é a do contrato: /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe responderam **404** em 'arena server', então o limite de pagamento e a leitura da carteira pela API são um achado (dono bootstrap/rel) e o baseline de carga mede as latências dessas respostas tratadas, não as do caminho de pagamento;
- o exercício mede o **piso** do processo: máquina local, dataset sintético e nenhum tráfego concorrente;
- a carga é o smoke versionado da P17-T06, não um teste de capacidade: os limites são modestos de propósito e não são promessa;
- a indisponibilidade do email é injetada no adapter de entrega, que é a fronteira do provedor dentro do processo; o que ela prova é a política do worker (registrar, manter, tentar de novo), que não muda com o fornecedor;
- o RPO observado é o do instante do desastre, não uma média: `archive_timeout` continua sendo o pior caso declarado;
- nenhum limiar foi reduzido, nenhum teste pulado, nenhum snapshot aceito, nenhum retry mascarado, nenhum waiver, nenhum `--force`/`--admin`/`--no-verify`;
- o drill é manual (`make disaster-drill`) e não está em `make verify`: ele exige Docker, k6 e navegador, e um gate de merge não é um release.

## 8. Fatos medidos (bloco executável)

O bloco abaixo é o que o portão lê. Ele é a forma mecânica do que este documento diz, e `make disaster-drill` recusa quando os dois discordam ou quando um número cruza um teto declarado.

```json
{
  "version": 1,
  "run_at": "2026-09-22",
  "commit": "59397d3",
  "host": {
    "kernel": "Linux 7.1.5-76070105-generic",
    "cpus": "16",
    "docker": "29.1.3"
  },
  "dataset": {
    "seed": "drill-168814",
    "accounts": 3,
    "ink": 1000,
    "arena": "drill-arena-drill-168814"
  },
  "baseline": {
    "ledger": {
      "captured_at": "2026-09-22 17:02:53.856754",
      "target": "arena@127.0.0.1:33077/arena",
      "server": "18.4 (Debian 18.4-1.pgdg13+1)",
      "archive_timeout_seconds": 300,
      "accounts": 3,
      "wallets": [
        {
          "account_id": "01a0ca11-fca2-7682-885c-42d440459487",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        },
        {
          "account_id": "01a0ca11-fd40-7ef9-9102-febf5f848c85",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        },
        {
          "account_id": "01a0ca11-fdde-7f54-bcf4-9c3351bf7e13",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        }
      ],
      "operations": 3,
      "transactions": 3,
      "ledger_digest": "5f70b120638d25a9597f83ea3ddb0a25",
      "balances_digest": "413c73014770c8e5b5e94c8a02ffc0f2",
      "free_ink": 3000,
      "purchased_ink": 0,
      "jobs": 0
    },
    "archive_failed": 0,
    "archive_lag_seconds": 1
  },
  "restored": {
    "ledger": {
      "captured_at": "2026-09-22 17:02:58.471287",
      "target": "arena@127.0.0.1:33078/arena",
      "server": "18.4 (Debian 18.4-1.pgdg13+1)",
      "archive_timeout_seconds": 0,
      "accounts": 3,
      "wallets": [
        {
          "account_id": "01a0ca11-fca2-7682-885c-42d440459487",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        },
        {
          "account_id": "01a0ca11-fd40-7ef9-9102-febf5f848c85",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        },
        {
          "account_id": "01a0ca11-fdde-7f54-bcf4-9c3351bf7e13",
          "stored_free": 1000,
          "stored_purchased": 0,
          "derived_free": 1000,
          "derived_purchased": 0,
          "rows": 1,
          "operations": 1
        }
      ],
      "operations": 3,
      "transactions": 3,
      "ledger_digest": "5f70b120638d25a9597f83ea3ddb0a25",
      "balances_digest": "413c73014770c8e5b5e94c8a02ffc0f2",
      "free_ink": 3000,
      "purchased_ink": 0,
      "jobs": 0
    },
    "archive_failed": 0,
    "archive_lag_seconds": -1
  },
  "timeline": {
    "baseline_at": "2026-09-22 17:02:53.856754",
    "target_at": "2026-09-22 17:02:54.916929",
    "disaster_at": "2026-09-22 17:02:56.551935",
    "rows_after_target": 0,
    "restore_command": "deploy/backup/restore.sh --base drill-2026-09-22T17-02-52Z --target-time 2026-09-22 17:02:54.916929",
    "primary_destroyed": true
  },
  "rpo": {
    "bound_seconds": 300,
    "observed_seconds": 5,
    "archive_lag_seconds": 1,
    "newest_recovered_commit": "2026-09-22 17:02:51.884752"
  },
  "rto": {
    "target_seconds": 14400,
    "to_writable_seconds": 2,
    "to_app_seconds": 3
  },
  "outage": {
    "email": {
      "attempts_before": 2,
      "error_code": "JOB_HANDLER_ERROR",
      "job_retained": true,
      "delivered_after": true,
      "delivered_in_seconds": 99
    },
    "stripe": {
      "routes": [
        {
          "what": "the provider webhook refuses an unverified event",
          "method": "POST",
          "path": "/api/v1/webhooks/stripe",
          "status": 404,
          "want": 400
        },
        {
          "what": "the checkout surface answers an unauthenticated caller",
          "method": "POST",
          "path": "/api/v1/me/billing/checkout",
          "status": 404,
          "want": 401
        },
        {
          "what": "the billing surface is mounted",
          "method": "GET",
          "path": "/api/v1/me/billing/subscription",
          "status": 404,
          "want": 401
        },
        {
          "what": "the wallet surface is mounted",
          "method": "GET",
          "path": "/api/v1/me/wallet",
          "status": 404,
          "want": 401
        }
      ],
      "verdict": "surface_absent",
      "ledger_rows_created": 0,
      "owner": "bootstrap/rel",
      "plan": "compose in 'arena server' the JSON surfaces the contract declares (the drill probed /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe) and register the Stripe webhook route; until then the payment boundary (and the wallet read of the restored data) is not reachable by the product"
    }
  },
  "load": {
    "tool": "k6",
    "script": "tests/load/smoke.js",
    "dataset": "drill-168814",
    "duration_seconds": 21,
    "thresholds": [
      {
        "metric": "http_req_failed",
        "bound": "rate\u003c0.05",
        "measured": "0"
      },
      {
        "metric": "http_req_duration{workload:cache-cold}",
        "bound": "p(95)\u003c1000",
        "measured": "0.3097252499999999"
      },
      {
        "metric": "http_req_duration{workload:cache-hot}",
        "bound": "p(95)\u003c300",
        "measured": "0.31246949999999996"
      },
      {
        "metric": "http_req_duration{workload:login}",
        "bound": "p(95)\u003c1500",
        "measured": "0.31890589999999996"
      },
      {
        "metric": "http_req_duration{workload:position}",
        "bound": "p(95)\u003c1000",
        "measured": "0.29713484999999995"
      },
      {
        "metric": "http_req_duration{workload:argument-wallet}",
        "bound": "p(95)\u003c1000",
        "measured": "0.27988155"
      },
      {
        "metric": "http_req_duration{workload:webhook-replay}",
        "bound": "p(95)\u003c1000",
        "measured": "0.3343214999999999"
      },
      {
        "metric": "http_req_duration{workload:arena-viral}",
        "bound": "p(95)\u003c500",
        "measured": "0.28227085"
      }
    ],
    "note": "as latências acima são as das respostas que a composição entregue dá aos caminhos do workload — /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe responderam 404 —, e não as de um caminho de pagamento ou de carteira montados: enquanto essas superfícies não estiverem compostas, o baseline mede o que existe, e o que existe não é o caminho que a fase pede para medir.",
    "breaches": []
  },
  "journeys": {
    "smoke": [
      {
        "what": "readiness on the restored data",
        "method": "GET",
        "path": "/health/ready",
        "status": 200,
        "want": 200
      },
      {
        "what": "the sign-in page",
        "method": "GET",
        "path": "/login",
        "status": 200,
        "want": 200
      },
      {
        "what": "the participation page of the Arena",
        "method": "GET",
        "path": "/arenas/drill-arena-drill-168814",
        "status": 200,
        "want": 200
      },
      {
        "what": "the participant signs in on the restored data",
        "method": "POST",
        "path": "/login",
        "status": 303,
        "want": 303
      },
      {
        "what": "a wrong password is refused",
        "method": "POST",
        "path": "/login",
        "status": 401,
        "want": 401
      },
      {
        "what": "the Arena as the signed-in participant reads it",
        "method": "GET",
        "path": "/arenas/drill-arena-drill-168814",
        "status": 200,
        "want": 200
      }
    ],
    "e2e": true,
    "e2e_detail": "the versioned harness drove its journeys green against the restored cluster (it provisions its own database inside it, because a journey's fixtures are not a customer's data: what the drill reads out of the restored data is the smoke probes and the ledger comparison above)"
  },
  "limitations": [
    "a superfície JSON que a composição entregue monta não é a do contrato: /api/v1/me/billing/checkout, /api/v1/me/billing/subscription, /api/v1/me/wallet, /api/v1/webhooks/stripe responderam **404** em 'arena server', então o limite de pagamento e a leitura da carteira pela API são um achado (dono bootstrap/rel) e o baseline de carga mede as latências dessas respostas tratadas, não as do caminho de pagamento;"
  ],
  "violations": null
}
```
