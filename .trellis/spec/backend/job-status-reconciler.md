# 作业状态读路径与集群对账器

> 适用于 `internal/apps/sync` 中 SeaTunnel 作业实例的观察面与后台对账。

## 原则

- **读路径**：`GET /api/v1/sync/jobs`、`GET /api/v1/sync/jobs/:id` 默认只读 `sync_job_instances`，禁止同步调用 SeaTunnel `/job-info`。
- **刷路径**：`StartJobReconciler` 按 `cluster_id` 分组，调用 `ListRunningJobs` / `ListFinishedJobs`，仅在状态变迁或必要时调用 `/job-info`。
- **手动刷新**：`GET /api/v1/sync/jobs/:id?fresh=1` 仍可对单条作业调用 `RefreshJobByID`（同步 `/job-info`），供运维或调试；CLI `--wait` 仍只轮询默认读路径。
- **失败语义**：问引擎失败只更新 `sync_state` / `sync_fail_count`，不得因一次 `/job-info` 失败把取消中作业写成 `canceled`。

## 观察字段

| 字段 | 含义 |
| --- | --- |
| `last_synced_at` | 最近一次成功对齐引擎的时间 |
| `sync_state` | `ok` / `degraded` / `stale` / `unknown` |
| `sync_fail_count` | 连续失败次数 |
| `sync_lag_ms` | API 派生：`now - last_synced_at` |

## 配置

`config.yaml` → `sync.job_reconciler_interval_seconds`：默认 10；设为 `-1` 关闭对账器。

## CLI

- 读作业：`stx sync job get <id>`（走默认读路径）
- 强制刷新：`GET ...?fresh=1`（codegen 路由可带 query）
- **`--wait`（submit/preview/recover）**：轮询 `GET /api/v1/sync/jobs/{id}`（读库，禁止 `fresh=1`）；默认 **follow 日志**，按 `offset`/`limit_bytes` 调 `GET .../logs`（Agent 读文件，不打 SeaTunnel REST）。
- stderr NDJSON 事件：`job_wait`、`job_status`（含 `sync_state`/`sync_lag_ms`）、`log_chunk`、`log_notice`、`job_observation_lost`；stdout 仍为终态 JSON。可用 `--no-follow-logs` 关闭日志跟随。
- **`--observation-timeout`**（默认 **3m**）：非终态且 `sync_state=stale` 持续超过该窗口（以 `sync_lag_ms` 或本地 stale 起始时间为准）时，`--wait` 以 timeout 退出并发出 `job_observation_lost`；设为 `0` 关闭。
- **集群不可用**：对账器抬高 `sync_fail_count` 并将 `sync_state` 置为 `degraded`/`stale`，**不**擅自把 `status` 改为 failed；Dolphin 应结合本参数与 DS `--timeout` 判定失败。
