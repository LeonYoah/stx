# 实施步骤：job-live-metrics-parity

1. 读 `frontend/components/common/sync/sync-studio-utils.ts` 中的 `extractJobMetricSummary`（约 :3264）、`countLogicalTables`（约 :3251）和 `normalizePairingTableKey`，列出回退顺序和逻辑表规则
2. 在 `internal/apps/sync/` 中查找能复用的函数：`collectTableMetricKeys`（job_metrics_log.go）、`tableMetricNumber`、`metricNumber`；如果已经有 `normalizePairingTableKey` 的 Go 等价实现（`grep -rn "Pairing\|logicalTable" internal/apps/sync`），直接复用
3. 改 `buildJobLiveMetrics`（job_list.go:342）：按 prd 的回退顺序取值，新增 `TableCount *int`
4. 在 `job_list_test.go` 中写表驱动测试（prd 中列出的 6 种场景）
5. 前端 `lib/services/sync/types.ts`：`SyncJobLiveMetrics.table_count?: number`
6. CLI：在 `internal/operation/registry_sync.go:60` 更新 `with_live` 的描述；如果 handler 的 Swagger 注释有变化，按项目惯例重新生成 `docs/docs.go`
7. 自检：`go test ./internal/apps/sync/... ./internal/operation/...`；本地执行 `stx sync job list --with_live 1`，确认多表作业有 `table_count`
8. 中文提交，例如 `fix(sync): 作业列表 live 指标回退口径与工作台对齐并新增 table_count`
