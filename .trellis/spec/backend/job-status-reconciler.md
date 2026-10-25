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

`config.yaml` → `sync.job_reconciler_event_mode_interval_seconds`：事件模式下完整对账的最长间隔，0 表示默认 60 秒，`-1` 关闭自适应。

## 事件模式（自适应轮询）

- 条件：集群事件通道为 `healthy`、`report_non_terminal=true`，并且集群版本 ≥3.0.0（见 [实时可观测性与事件上报](./engine-events-and-realtime.md)）。
- 满足时，在完整对账间隔内每轮只做 `reconcileClusterJobsLight`：**不拉 running/finished 列表**，只给 `running` 作业中"30 秒内被读过"（`GetJob`/`GetJobLogs`/`/realtime` 会打标记）或 `last_synced_at` 超过 30 秒的作业拉 `/job-info`。
- 状态变化靠事件触发 `RefreshJobByID`（防抖 1 秒）；非 running 的非终态作业最多等一个完整对账间隔，由完整对账兜底。
- 通道降级、未开启、2.3.x、或配置为 `-1` 时，每轮都是完整对账，行为和原来一致。作业从引擎消失时置为 failed、终态补拉终态指标等语义**只在完整对账里发生**。
- `lastFull` 只存在内存里：STX 重启后第一轮必然是完整对账。CLI `--wait` 轮询 `GetJob` 会让作业保持高频拉取。

## CLI

- 读作业：`stx sync job get <id>`（走默认读路径）
- 强制刷新：`GET ...?fresh=1`（codegen 路由可带 query）
- **`--wait`（submit/preview/recover）**：轮询 `GET /api/v1/sync/jobs/{id}`（读库，禁止 `fresh=1`）；默认 **follow 日志**，按 `offset`/`limit_bytes` 调 `GET .../logs`（Agent 读文件，不打 SeaTunnel REST）。
- stderr NDJSON 事件：`job_wait`、`job_status`（含 `sync_state`/`sync_lag_ms`）、`log_chunk`、`log_notice`、`job_observation_lost`；stdout 仍为终态 JSON。可用 `--no-follow-logs` 关闭日志跟随。
- **`--observation-timeout`**（默认 **3m**）：非终态且 `sync_state=stale` 持续超过该窗口（以 `sync_lag_ms` 或本地 stale 起始时间为准）时，`--wait` 以 timeout 退出并发出 `job_observation_lost`；设为 `0` 关闭。
- **集群不可用**：对账器抬高 `sync_fail_count` 并将 `sync_state` 置为 `degraded`/`stale`，**不**擅自把 `status` 改为 failed；Dolphin 应结合本参数与 DS `--timeout` 判定失败。
- **作业从引擎消失**：集群 API 可达，但 running/finished 列表都没有该 `engine_job_id`，且 `/job-info` 仅返回空壳 `{jobId}`（常见于集群重启或历史过期）时，对账器将作业置为 `failed`，避免永久 `running` 僵尸。手动提交、内置 schedule、Dolphin `external_ref` 只要落了 `engine_job_id` 都走同一条路径。
- **日志换行**：Agent/REST/CLI 传输路径必须保留真实换行；`log_chunk` 同时带 `lines`，并额外发 `log_line` 便于调度器任务日志按行阅读。
- **指标入日志流**：对账写入 `result_preview.metrics` 后，每约 30s（及终态）追加可读指标块；`GET .../logs` 返回 `metrics_log`；CLI `--wait` 同步打印。
