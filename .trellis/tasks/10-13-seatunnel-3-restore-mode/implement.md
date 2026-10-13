# Implement: restoreMode 接入

## 顺序

1. **Go 核心**
   - [ ] `resolveRestorePlan(source *JobInstance, engineVersion string) (platformJobID, restoreMode, restoreSourceJobID, legacySavepoint bool)`
   - [ ] 扩展 `EngineSubmitRequest` + `engine_client.Submit` query 参数
   - [ ] `RecoverJob` / `submitTaskInstanceWithPayload` 接入 plan；CHECKPOINT 用 `nextJobID()`
   - [ ] `submit_spec` 记录 `restore_mode`、`restore_source_job_id`（中英注释）
   - [ ] `engine_client_test`：参数组装单测

2. **Go 服务单测**
   - [ ] `TestRecoverJob_*`：3.0 FAILED → CHECKPOINT 新 jobId；SAVEPOINT_DONE → 同源；2.x → legacy

3. **前端**
   - [ ] `resolveRecoverModeLabel(job)` 与 Go 规则对齐（或消费 API 返回的 restore_mode）
   - [ ] 恢复按钮 / pending 文案 i18n（zh/en）
   - [ ] 可选：recover API 响应带 `restore_mode` 供 UI

4. **CLI**
   - [ ] 更新 `newSyncJobRecoverCommand` Long/Example 文案（无新 flag）

5. **验证**
   - [ ] `go test ./internal/apps/sync/...`
   - [ ] `cd frontend && pnpm exec vitest run`（若有 utils 单测）
   - [ ] 手工：3.0 FakeSource 失败 → recover；savepoint stop → recover

## 关键文件

- `internal/apps/sync/engine_client.go`
- `internal/apps/sync/service.go`
- `internal/apps/sync/service_test.go`
- `frontend/components/common/sync/sync-studio-utils.ts`
- `frontend/components/common/sync/DataSyncStudio.tsx` / `ConsolePanel.tsx`
- `internal/cmd/sync.go`
- `frontend/lib/i18n/locales/{zh,en}.json`

## Checklist（spec）

- 后端：`.trellis/spec/backend/index.md` 错误处理、日志
- 跨层：`.trellis/spec/guides/cross-layer-thinking-guide.md` jobId 契约
- CLI 约定：AGENTS.md — 行为变更更新 recover 帮助文案
