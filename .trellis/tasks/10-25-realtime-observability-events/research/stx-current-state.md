# STX 现状：作业状态、指标与可复用资产

> 调研时间：2026-10-25，基于分支 `fix/admin-self-disable-guard`（`97ffcc9`）。

## 状态与指标链路

- 对账器 `internal/apps/sync/job_reconciler.go`：
  - 默认每 10 秒一轮（`sync.job_reconciler_interval_seconds`）。
  - `reconcileClusterJobs`（`:125`）每个集群调 1 次 `ListRunningJobs`，再按状态循环 6 次 `ListFinishedJobs`（`:167`）。
  - 运行中作业每个都调一次 `GetJobInfo`，并写入 `result_preview.metrics` 和 `job_dag`（`mergeJobRuntimeInfo`，`service.go:2485`）。
- `job_metrics_log.go`：每 30 秒（以及终态时）往 `result_preview.stx_metrics_log` 追加一个文本块，上限 64KB。
- 读路径：`GetJob` 只读库（`service.go:1645`）；`?fresh=1` → `RefreshJobByID` → `refreshJobInstance`。spec 见 `.trellis/spec/backend/job-status-reconciler.md`。
- 前端：`DataSyncStudio.tsx:1685` 每 3 秒对活跃作业调 `getJob`；指标弹窗 `StudioDialogs.tsx:365` `MetricsDialogContent` 只显示快照；作业 DAG 弹窗使用 `result_preview.job_dag`（`DataSyncStudio.tsx:5313`）配合 `WebUiDagPreview`。
- 前端已有 recharts（`frontend/package.json`）。

## 可复用资产

| 需求 | 可复用 | 位置 |
| --- | --- | --- |
| 集群版本 | `ClusterVersionProvider.GetClusterVersion` | `service.go:63`，router 注入 `clusterService` |
| 版本比较和能力 | `seatunnel.CompareVersions`、`CapabilitiesForVersion` | `internal/seatunnel/capabilities.go` |
| 引擎地址 | `DefaultClusterRuntimeResolver.ResolveEngineEndpoint` 取**第一个在线节点** | `engine_client.go:496`（实时接口需要新增"列出全部候选节点"） |
| 任务流式/批量 | `Task.Mode`（`streaming`/`batch`） | `model.go:183`；`TaskModeStreaming` |
| 任务设置存储 | `Task.Definition` JSONMap（例如 `execution_mode`） | 前端 `getExecutionMode`（`sync-studio-utils.ts:1848`）、`handleExecutionModeChange` |
| 提交链路 | `submitTaskInstanceWithPayload`（运行、恢复、定时都会经过），`SubmitSpec.submitted_content` 记录实际提交内容 | `service.go:1571` |
| 回调鉴权 | 用 `SessionSecret` 做 HMAC 的 `buildPreviewCollectToken` / `validatePreviewCollectAuthToken` | `service.go:2308`、`handler.go:717` |
| 公开回调路由 | `apiV1Router.POST("/sync/preview/collect", ...)`，并在登记表里标为 `ModeServerOnly` | `router.go:748`、`internal/operation/registry.go:2298` |
| CLI | 在操作登记表 `syncOp(...)` 加条目，命令自动生成 | `internal/operation/registry_sync.go` |
| 外部访问地址 | `config.GetExternalURL()`（`app.external_url`） | `internal/config/config.go:394` |
| seatunnel.yaml 管理 | 每个集群有模板和节点配置：`Update` 先推送再提交新版本，推送失败时标记待推送；`findYAMLMapChild` 用 yaml.Node 改指定键 | `internal/apps/config/service.go:179`、`:570` |
| 滚动重启 | #65 扩容计划里的滚动重启 | `internal/apps/cluster` |
| 告警 | 平台健康和 Prometheus 两类模板（P3 才用） | `internal/apps/monitoring` |

## 注意点

- `EngineClient` 接口（`engine_client.go:40`）有很多测试桩，新增能力用可选接口加类型断言，不扩展原接口。
- `ClusterRuntimeResolver` 也有测试桩，同样用可选接口 `ClusterEngineEndpointLister`。
- 登记表要求每条路由都有归属（CLI 操作或 ServerOnly 等），新增路由要同步登记。
- 敏感信息：事件 token 不落库（每次用 HMAC 算出来）；`READER_MESSAGE_DELAYED.record` 必须删除，不能落库也不能打日志。
