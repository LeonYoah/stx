# 后端作业列表 live 指标口径对齐工作台

> 父任务：`11-17-unified-job-table`（全局契约见父任务 design.md §3.3）

## 目标

`GET /sync/jobs?with_live=1` 返回的 `live` 是作业列表指标的唯一来源，它的取值口径必须和工作台 `extractJobMetricSummary`（`frontend/components/common/sync/sync-studio-utils.ts:3264`）一致，并补充逻辑表数量。

## 现状问题

`internal/apps/sync/job_list.go#buildJobLiveMetrics` 只读这四个指标：`SourceReceivedCount`、`SinkWriteCount`、`SourceReceivedQPS`、`SinkWriteQPS`。SeaTunnel 3.0 多表作业只有表级指标，或者只有 `SinkCommitted*` 指标，这时大盘显示 `-`，而工作台能显示出数值。

## 需求

1. 回退顺序和前端保持一致：
   - `write_total`：`SinkWriteCount` → `SinkCommittedCount` → 表级 `TableSinkWriteCount` 求和 → 表级 `TableSinkCommittedCount` 求和
   - `read_total`：`SourceReceivedCount` → 表级 `TableSourceReceivedCount` 求和
   - `write_qps`：`SinkWriteQPS` → `SinkCommittedQPS` → 表级 `TableSinkWriteQPS` 求和（仅运行中）
   - `read_qps`：`SourceReceivedQPS` → 表级 `TableSourceReceivedQPS` 求和（仅运行中）
   - 实现前逐行对照 `extractJobMetricSummary` 的实际代码，**以前端现有行为为准**；上面的顺序和前端不一致时，按前端改，并在本文件更正。
2. 新增 `table_count`（`*int`，`omitempty`）：从 `TableSourceReceivedCount` / `TableSinkWriteCount` / `TableSinkCommittedCount` 收集键（可复用 `job_metrics_log.go#collectTableMetricKeys`），按逻辑表去重。去重规则移植前端的 `countLogicalTables` / `normalizePairingTableKey`，保证 `Source[0].db.t` 和 `Sink[0].db.t` 算同一张表。只有 ≥1 时才输出。
3. 「只有运行中才有速率 / CDC」的现有约束保持不变。
4. 前端类型 `SyncJobLiveMetrics` 加 `table_count?: number`，并补上中英双语注释。
5. CLI：`internal/operation/registry_sync.go` 中 `with_live` 的说明更新为「包含任务名与 live 读写速率、条数、逻辑表数」（英文）。如果改了 Swagger 注释，重新生成 `docs/docs.go`。

## 验收标准

- [ ] 表驱动单测覆盖以下场景：仅全局指标、仅 `SinkCommitted*`、仅表级指标（多表）、`Source[0]` / `Sink[0]` 同名表去重、非运行中不返回 qps、空指标返回 nil
- [ ] 为前端 `extractJobMetricSummary` 准备一组相同的 fixture，前后端算出的 read/write count 一致（在 Go 测试的注释里注明 fixture 来源）
- [ ] `go test ./internal/apps/sync/... ./internal/operation/...` 通过
- [ ] `stx sync job list --with_live 1` 的输出里能看到 `table_count`
