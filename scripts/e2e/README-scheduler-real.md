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
