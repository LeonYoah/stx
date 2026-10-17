# Real scheduler E2E (DolphinScheduler ↔ STX)

## What it runs

GitHub Actions job **E2E Real Scheduler** boots:

1. Installer-real stack (STX + Agent + SeaTunnel package cache) — same as Workbench
2. `apache/dolphinscheduler-standalone-server:3.2.2` on host network with `dist/stx` bind-mounted
3. Playwright `scheduler-real.spec.ts`: publish FakeSource success + Sql-fail tasks, push SHELL workflows, start them, assert **real DS task logs**

## Pitfalls avoided

| Pitfall | Mitigation |
|---------|------------|
| paths-filter skip | Explicit `scheduler_real:` key with concrete paths (not only shared globs) |
| DS H2 wipe on restart | Fresh container each run; bootstrap token/project/env every time |
| Bind-mount stale inode | Start DS **after** `go build -o dist/stx` |
| glibc mismatch (CI 24.04 → DS 22.04) | `CGO_ENABLED=0 go build` so `stx` is static; `docker exec … stx version` before Playwright |
| smoke picks up `*-real` specs | `scheduler-real.spec.ts` in `smokeExcludedSpecs` |
| job-guard `setsid` → exit 9 | DS `--security-opt seccomp=unconfined` + CLI setsid fallback |
| HOCON `fields { a = x b = y }` | Multi-line `fields { }` (Workbench pattern); single-line → ST 500 |
| EnvironmentCode | Create env with `export STX_SERVER=...` / `export STX_TOKEN=...`; push `--environment stx-cli-prod` |
| Fail stuck RUNNING_EXECUTION | Long wait; still read task logPath from container |
| SeaTunnel package | Reuse `prepare-package` artifact / cache |
| Ports | Installer `pick_port`; DS `--network host` → STX on `127.0.0.1` |

## Local

```bash
# Requires Docker + Go + pnpm; pulls DS 3.2.2 and SeaTunnel package.
E2E_INSTALLER_REAL_PACKAGE_CACHE_DIR=$PWD/tmp/e2e-package-cache \
  bash frontend/scripts/e2e/run-real-scheduler.sh
```

Contract suite (`run-scheduler-contract.sh`) remains the fast gate; this job is the fidelity check.

## Wall-clock target

| Segment | Budget (warm CI) |
|---------|------------------|
| Setup (Go/Node/pnpm/Playwright/ST package) | ~3–4 min |
| STX+Agent boot (no MinIO) + static `stx` + DS start | ~2 min |
| Cluster install + FakeSource success/fail via DS | ~2–3 min |
| **Job total** | **≤8–10 min** (hard cap 20 min) |

Why not merge into Workbench: Workbench already ~7 min of UI flows; bolting DS onto it would couple failures and still serialize. Separate job + SeaTunnel artifact reuse is the compromise.

Speed levers already applied: `CGO_ENABLED=0` stx, DS image GHA cache, skip MinIO, `FailRetryTimes=0` on pushed SHELL nodes, no Playwright retry, 3 min DS wait cap.
