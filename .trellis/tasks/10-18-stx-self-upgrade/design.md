# Design: STX Console Self-Upgrade

## Scope
Console-admin self-upgrade of STX control plane + Agent + Proxy jars. Distinct from SeaTunnel cluster `stupgrade`.

## Backend (`internal/apps/selfupgrade`)
- In-process state machine + filesystem staging dir under `{data_dir}/self-upgrade/`.
- Optional DB task record for audit/history (AutoMigrate).
- Check: GitHub releases/latest; local mock when `STX_SELF_UPGRADE_MOCK=1` or `simulate_failure`.
- Stage: write assets; checksum against SHA256SUMS when present.
- Execute: dry-apply MVP (validate + sequenced steps); real binary replace stubbed with clear "apply" hooks for follow-up.
- Concurrency lock: single active stage/execute.

## Frontend
- Route: `/system-upgrade` (admin).
- Profile card: 「检查更新」→ navigate; non-admin disabled + tooltip.
- WorkspaceHeader + Cards + Tabs (online|offline) + step scope list; zero-collapse skeletons.
- i18n: `selfUpgrade` namespace in zh/en.
