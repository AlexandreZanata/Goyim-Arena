# P17-T06 load smoke

`smoke.js` is the versioned, synthetic workload for the first capacity baseline. It
runs these workloads in separate windows so the report can identify their
latencies without mixing budgets:

- public Arena cache cold/hot reads;
- login;
- position read;
- argument plus wallet read;
- Stripe webhook replay handling;
- viral Arena read fan-out.

The dataset is selected by `K6_DATASET_SEED` and resource identifiers are passed
explicitly. No production account, token, webhook secret, or fixture is stored
in this repository.

Run against a local instance prepared with synthetic data only:

```bash
K6_BASE_URL=http://127.0.0.1:8080 \
K6_DATASET_SEED=synthetic-p17-t06 \
K6_ARENA_ID=<synthetic-arena-id> \
K6_ACCOUNT_EMAIL=load-test@example.invalid \
K6_ACCOUNT_PASSWORD='<synthetic-password>' \
make test-load-smoke
```

The command records the current commit, host/kernel/CPU information, the
configured dataset seed, and k6's JSON summary. Thresholds are intentionally
modest first-stage baselines; they are not a capacity promise.
