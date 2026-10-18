# Scheduler contract E2E (CI-light)

## Goal

Prove the **DolphinScheduler ↔ STX CLI** behaviors we validated manually:

1. `stx sync task submit --wait` **success** and **failure**
2. Default stderr is **plain human text** (no NDJSON `{"event":...}`)
3. **Final metrics** blocks appear on terminal
4. Key **status mapping** (FAILING / DOING_SAVEPOINT / SAVEPOINT_DONE, …)
5. Pushed DS SHELL script keeps `--wait --confirm` + `external-ref`

## Tradeoff

| Approach | Pros | Cons |
|----------|------|------|
| **This suite (httptest + Go)** | Fast (~seconds), deterministic, runs on every relevant PR path | Does not boot real DS / SeaTunnel |
| Full DS + SeaTunnel in Actions | Highest fidelity | Heavy images, flaky, long wall time |

Real DS+ST smoke stays a **manual / local** path (see `/workspace/tzrun/smoke/ds-*-human.log` notes on working branches). CI gates the CLI/API **contract** that DS SHELL depends on.

## Run locally

```bash
./scripts/e2e/run-scheduler-contract.sh
```

## Wiring

GitHub Actions job **E2E Scheduler Contract** in `.github/workflows/e2e.yml`, scoped by `Detect E2E Scope` filter `scheduler_contract`.
